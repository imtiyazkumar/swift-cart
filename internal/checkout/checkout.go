package checkout

import (
    "context"
    "errors"
    "fmt"
    "github.com/imtiyazkumar/yember-core/internal/stations"
    "os"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Service struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Service { return &Service{db: db} }

type CartItem struct {
    ID        string `json:"id"`
    ItemID    string `json:"item_id"`
    Name      string `json:"name"`
    Quantity  int    `json:"quantity"`
    UnitPrice int64  `json:"unit_price"`
    Currency  string `json:"currency"`
}
type Cart struct {
    MerchantID string     `json:"merchant_id"`
    Items      []CartItem `json:"items"`
    Subtotal   int64      `json:"subtotal"`
    Currency   string     `json:"currency"`
}
type OrderResult struct {
    ID       string `json:"id"`
    Status   string `json:"status"`
    Subtotal int64  `json:"subtotal"`
    Currency string `json:"currency"`
}

type OrderListItem struct {
    ID           string    `json:"id"`
    Status       string    `json:"status"`
    MerchantName string    `json:"merchant_name"`
    Subtotal     int64     `json:"subtotal"`
    Currency     string    `json:"currency"`
    CreatedAt    time.Time `json:"created_at"`
}

func (s *Service) Add(ctx context.Context, customerID, itemID string, quantity int, addressIDs ...string) error {
    if quantity < 1 || quantity > 99 {
        return errors.New("quantity must be between 1 and 99")
    }
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return err
    }
    var lockedID string
    if err = tx.QueryRow(ctx, `SELECT id FROM public.users WHERE id=$1 FOR UPDATE`, customerID).Scan(&lockedID); err != nil {
        return err
    }
    if len(addressIDs) != 1 || addressIDs[0] == "" {
        return errors.New("select a delivery address before adding items")
    }
    station, err := stations.OwnedAddress(ctx, tx, customerID, addressIDs[0])
    if err != nil {
        return err
    }
    var merchantID string
    var currency string
    err = tx.QueryRow(ctx, `SELECT merchant_id,currency FROM catalog.items WHERE id=$1 AND is_available AND deleted_at IS NULL`, itemID).Scan(&merchantID, &currency)
    if err != nil {
        return errors.New("item is unavailable")
    }
    var eligible bool
    if err = tx.QueryRow(ctx, `SELECT station_id=$2 AND COALESCE(public.station_covers(station_id,latitude,longitude),false) AND is_active AND approval_status='APPROVED' AND deleted_at IS NULL FROM public.merchants WHERE id=$1 FOR SHARE`, merchantID, station).Scan(&eligible); err != nil || !eligible {
        return errors.New("this restaurant does not deliver to the selected address")
    }
    if currency != "INR" {
        return errors.New("only INR items can be checked out")
    }
    var cartID, currentMerchant string
    err = tx.QueryRow(ctx, `SELECT id, merchant_id FROM orders.carts WHERE customer_id=$1 AND status='ACTIVE' LIMIT 1 FOR UPDATE`, customerID).Scan(&cartID, &currentMerchant)
    if err == pgx.ErrNoRows {
        err = tx.QueryRow(ctx, `INSERT INTO orders.carts(customer_id, merchant_id, status) VALUES($1,$2,'ACTIVE') RETURNING id`, customerID, merchantID).Scan(&cartID)
    } else if err == nil && currentMerchant != merchantID {
        return errors.New("cart already contains items from another restaurant; clear it before switching")
    }
    if err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `UPDATE orders.carts SET delivery_address_id=$2 WHERE id=$1`, cartID, addressIDs[0]); err != nil {
        return err
    }
    // The cart row is locked above, and the customer row is locked for the
    // duration of this transaction. Update an existing line first so adding to
    // a cart does not depend on a unique index being present in every database.
    tag, err := tx.Exec(ctx, `UPDATE orders.cart_items
        SET quantity=quantity+$3, unit_price=(SELECT base_price FROM catalog.items WHERE id=$2)
        WHERE cart_id=$1 AND item_id=$2`, cartID, itemID, quantity)
    if err != nil {
        return err
    }
    if tag.RowsAffected() > 0 {
        var updatedQuantity int
        if err = tx.QueryRow(ctx, `SELECT quantity FROM orders.cart_items WHERE cart_id=$1 AND item_id=$2`, cartID, itemID).Scan(&updatedQuantity); err != nil {
            return err
        }
        if updatedQuantity > 99 {
            return errors.New("item quantity cannot exceed 99")
        }
    } else {
        tag, err = tx.Exec(ctx, `INSERT INTO orders.cart_items(cart_id,item_id,quantity,unit_price)
            SELECT $1,id,$2,base_price FROM catalog.items WHERE id=$3 AND is_available AND deleted_at IS NULL`, cartID, quantity, itemID)
        if err != nil {
            return err
        }
    }
    if tag.RowsAffected() == 0 {
        return errors.New("item is unavailable")
    }
    _, err = tx.Exec(ctx, `UPDATE orders.carts SET updated_at=now() WHERE id=$1`, cartID)
    if err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (s *Service) Get(ctx context.Context, customerID string) (*Cart, error) {
    var cart Cart
    cart.Items = []CartItem{}
    var cartID string
    err := s.db.QueryRow(ctx, `SELECT id,merchant_id FROM orders.carts WHERE customer_id=$1 AND status='ACTIVE' ORDER BY updated_at DESC LIMIT 1`, customerID).Scan(&cartID, &cart.MerchantID)
    if err == pgx.ErrNoRows {
        return &cart, nil
    }
    if err != nil {
        return nil, err
    }
    rows, err := s.db.Query(ctx, `SELECT ci.id,ci.item_id,i.name,ci.quantity,i.base_price,i.currency FROM orders.cart_items ci JOIN catalog.items i ON i.id=ci.item_id WHERE ci.cart_id=$1 ORDER BY ci.id`, cartID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    for rows.Next() {
        var item CartItem
        if err = rows.Scan(&item.ID, &item.ItemID, &item.Name, &item.Quantity, &item.UnitPrice, &item.Currency); err != nil {
            return nil, err
        }
        if cart.Currency == "" {
            cart.Currency = item.Currency
        }
        cart.Subtotal += item.UnitPrice * int64(item.Quantity)
        cart.Items = append(cart.Items, item)
    }
    return &cart, rows.Err()
}

func (s *Service) ListOrders(ctx context.Context, customerID string) ([]OrderListItem, error) {
    rows, err := s.db.Query(ctx, `
        SELECT o.id, o.status, m.name, ROUND(o.total_price * 100)::BIGINT,
               'INR', o.created_at
        FROM public.orders o
    JOIN public.merchants m ON m.id = o.merchant_id
        WHERE o.customer_id = $1
        ORDER BY o.created_at DESC
        LIMIT 100`, customerID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    orders := make([]OrderListItem, 0)
    for rows.Next() {
        var item OrderListItem
        if err := rows.Scan(&item.ID, &item.Status, &item.MerchantName, &item.Subtotal, &item.Currency, &item.CreatedAt); err != nil {
            return nil, err
        }
        orders = append(orders, item)
    }
    return orders, rows.Err()
}

func (s *Service) SetQuantity(ctx context.Context, customerID, itemID string, quantity int) error {
    if quantity < 0 || quantity > 99 {
        return errors.New("quantity must be between 0 and 99")
    }
    if quantity == 0 {
        _, err := s.db.Exec(ctx, `DELETE FROM orders.cart_items ci USING orders.carts c WHERE ci.cart_id=c.id AND ci.id=$1 AND c.customer_id=$2 AND c.status='ACTIVE'`, itemID, customerID)
        return err
    }
    ct, err := s.db.Exec(ctx, `UPDATE orders.cart_items ci SET quantity=$1,unit_price=i.base_price FROM orders.carts c,catalog.items i WHERE ci.cart_id=c.id AND ci.item_id=i.id AND ci.id=$2 AND c.customer_id=$3 AND c.status='ACTIVE' AND i.is_available AND i.deleted_at IS NULL`, quantity, itemID, customerID)
    if err == nil && ct.RowsAffected() == 0 {
        return errors.New("cart item not found or unavailable")
    }
    return err
}

func (s *Service) Clear(ctx context.Context, customerID string) error {
    _, err := s.db.Exec(ctx, `DELETE FROM orders.carts WHERE customer_id=$1 AND status='ACTIVE'`, customerID)
    return err
}

func (s *Service) Checkout(ctx context.Context, customerID, addressID, idempotencyKey string) (*OrderResult, error) {
    if os.Getenv("CUSTOMER_PAYMENT_METHOD") != "CASH_ON_DELIVERY" {
        return nil, errors.New("customer payment method is not configured; this release supports configured CASH_ON_DELIVERY orders")
    }
    if idempotencyKey == "" {
        return nil, errors.New("idempotency_key is required")
    }
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return nil, err
    }
    if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, customerID+":"+idempotencyKey); err != nil {
        return nil, err
    }
    var lockedCustomerID string
    if err = tx.QueryRow(ctx, `SELECT id FROM public.users WHERE id=$1 FOR UPDATE`, customerID).Scan(&lockedCustomerID); err != nil {
        return nil, err
    }
    var prior OrderResult
    err = tx.QueryRow(ctx, `SELECT id,status,ROUND(total_price*100)::BIGINT,'INR' FROM public.orders WHERE customer_id=$1 AND idempotency_key=$2`, customerID, idempotencyKey).Scan(&prior.ID, &prior.Status, &prior.Subtotal, &prior.Currency)
    if err == nil {
        return &prior, nil
    }
    if err != pgx.ErrNoRows {
        return nil, err
    }
    stationID, err := stations.OwnedAddress(ctx, tx, customerID, addressID)
    if err != nil {
        return nil, err
    }
    var address []byte
    err = tx.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'label',label,'address_line1',address_line1,'address_line2',address_line2,'city',city,'state',state,'postal_code',postal_code,'latitude',latitude,'longitude',longitude) FROM users.customer_addresses WHERE id=$1 AND customer_id=$2 AND deleted_at IS NULL`, addressID, customerID).Scan(&address)
    if err != nil {
        return nil, errors.New("delivery address not found")
    }
    var cartID, merchantID string
    err = tx.QueryRow(ctx, `SELECT id,merchant_id FROM orders.carts WHERE customer_id=$1 AND status='ACTIVE' FOR UPDATE`, customerID).Scan(&cartID, &merchantID)
    if err != nil {
        return nil, errors.New("cart is empty")
    }
    var totalLines, availableLines int
    var restaurantEligible bool
    if err = tx.QueryRow(ctx, `SELECT station_id=$2 AND COALESCE(public.station_covers(station_id,latitude,longitude),false) AND is_active AND approval_status='APPROVED' AND deleted_at IS NULL FROM public.merchants WHERE id=$1 FOR SHARE`, merchantID, stationID).Scan(&restaurantEligible); err != nil || !restaurantEligible {
        return nil, errors.New("restaurant is closed, unapproved, or does not deliver to the selected address")
    }
    if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM orders.cart_items WHERE cart_id=$1`, cartID).Scan(&totalLines); err != nil {
        return nil, err
    }
    if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM orders.cart_items ci JOIN catalog.items i ON i.id=ci.item_id WHERE ci.cart_id=$1 AND i.is_available AND i.deleted_at IS NULL`, cartID).Scan(&availableLines); err != nil {
        return nil, err
    }
    if totalLines == 0 || availableLines != totalLines {
        return nil, errors.New("one or more cart items are no longer available")
    }
    lockedItems, err := tx.Query(ctx, `SELECT i.id FROM orders.cart_items ci JOIN catalog.items i ON i.id=ci.item_id WHERE ci.cart_id=$1 ORDER BY i.id FOR UPDATE OF ci FOR SHARE OF i`, cartID)
    if err != nil {
        return nil, err
    }
    for lockedItems.Next() {
        var lockedItemID string
        if err = lockedItems.Scan(&lockedItemID); err != nil {
            lockedItems.Close()
            return nil, err
        }
    }
    if err = lockedItems.Err(); err != nil {
        lockedItems.Close()
        return nil, err
    }
    lockedItems.Close()
    var subtotal int64
    err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(i.base_price*ci.quantity),0) FROM orders.cart_items ci JOIN catalog.items i ON i.id=ci.item_id WHERE ci.cart_id=$1 AND i.is_available AND i.deleted_at IS NULL`, cartID).Scan(&subtotal)
    if err != nil {
        return nil, err
    }
    if subtotal <= 0 {
        return nil, errors.New("cart is empty or items unavailable")
    }
    var orderID string
    err = tx.QueryRow(ctx, `INSERT INTO public.orders(merchant_id,customer_id,status,total_price,idempotency_key,delivery_address,station_id) VALUES($1,$2,'CREATED',$3::numeric/100,$4,$5::jsonb,$6) RETURNING id`, merchantID, customerID, subtotal, idempotencyKey, string(address), stationID).Scan(&orderID)
    if err != nil {
        return nil, err
    }
    _, err = tx.Exec(ctx, `INSERT INTO orders.order_items(order_id,item_id,name_snapshot,quantity,unit_price,total_price) SELECT $1,i.id,i.name,ci.quantity,i.base_price,i.base_price*ci.quantity FROM orders.cart_items ci JOIN catalog.items i ON i.id=ci.item_id WHERE ci.cart_id=$2 AND i.is_available AND i.deleted_at IS NULL`, orderID, cartID)
    if err != nil {
        return nil, err
    }
    _, err = tx.Exec(ctx, `INSERT INTO orders.order_status_history(order_id,status,changed_by) VALUES($1,'CREATED',$2)`, orderID, customerID)
    if err != nil {
        return nil, err
    }
    _, err = tx.Exec(ctx, `UPDATE orders.carts SET status='CHECKED_OUT',updated_at=now() WHERE id=$1`, cartID)
    if err != nil {
        return nil, err
    }
    if err = tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("checkout transaction failed: %w", err)
    }
    return &OrderResult{ID: orderID, Status: "CREATED", Subtotal: subtotal, Currency: "INR"}, nil
}

func (s *Service) AddAddress(ctx context.Context, customerID, label, line1, line2, city, state, postal string, lat, lon *float64) (string, error) {
    if line1 == "" || city == "" || state == "" || postal == "" {
        return "", errors.New("address_line1, city, state, and postal_code are required")
    }
    if lat != nil || lon != nil {
        if err := stations.Coordinates(lat, lon); err != nil {
            return "", err
        }
    }
    var id string
    err := s.db.QueryRow(ctx, `INSERT INTO users.customer_addresses(customer_id,label,address_line1,address_line2,city,state,postal_code,latitude,longitude) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9) RETURNING id`, customerID, label, line1, line2, city, state, postal, lat, lon).Scan(&id)
    return id, err
}

// SelectAddress re-resolves coverage and removes incompatible cart items atomically.
func (s *Service) SelectAddress(ctx context.Context, customer, address string) (string, bool, error) {
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return "", false, err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return "", false, err
    }
    var locked string
    if err = tx.QueryRow(ctx, `SELECT id FROM public.users WHERE id=$1 FOR UPDATE`, customer).Scan(&locked); err != nil {
        return "", false, err
    }
    var owned bool
    if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users.customer_addresses WHERE id=$1 AND customer_id=$2 AND deleted_at IS NULL)`, address, customer).Scan(&owned); err != nil {
        return "", false, err
    }
    if !owned {
        return "", false, errors.New("delivery address not found")
    }
    station, coverageErr := stations.OwnedAddress(ctx, tx, customer, address)
    var cart, merchant string
    err = tx.QueryRow(ctx, `SELECT id,merchant_id FROM orders.carts WHERE customer_id=$1 AND status='ACTIVE' FOR UPDATE`, customer).Scan(&cart, &merchant)
    cleared := false
    if err != nil && err != pgx.ErrNoRows {
        return "", false, err
    }
    if err == nil {
        valid := false
        if coverageErr == nil {
            if err = tx.QueryRow(ctx, `SELECT station_id=$2 AND COALESCE(public.station_covers(station_id,latitude,longitude),false) FROM public.merchants WHERE id=$1 FOR SHARE`, merchant, station).Scan(&valid); err != nil {
                return "", false, err
            }
        }
        if !valid {
            if _, err = tx.Exec(ctx, `DELETE FROM orders.carts WHERE id=$1`, cart); err != nil {
                return "", false, err
            }
            cleared = true
        } else {
            if _, err = tx.Exec(ctx, `UPDATE orders.carts SET delivery_address_id=$2 WHERE id=$1`, cart, address); err != nil {
                return "", false, err
            }
        }
    }
    if err = tx.Commit(ctx); err != nil {
        return "", false, err
    }
    return station, cleared, coverageErr
}

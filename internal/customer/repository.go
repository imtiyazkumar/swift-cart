package customer

import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

type Repository interface {
    CreateCustomer(ctx context.Context, c *Customer) (string, error)
    GetCustomerByID(ctx context.Context, id string) (*Customer, error)
    ListCustomers(ctx context.Context) ([]*Customer, error)
    UpdateCustomer(ctx context.Context, c *Customer) error
    DeleteCustomer(ctx context.Context, id string) error

    // Ecosystem Restaurant & Discovery Methods
    ListRestaurants(ctx context.Context, city, search string) ([]*Restaurant, error)
    GetRestaurantByID(ctx context.Context, id string) (*Restaurant, error)

    // Favorites & Coupons & Reviews & Notifications
    ToggleFavorite(ctx context.Context, customerID, merchantID string) (bool, error)
    GetFavorites(ctx context.Context, customerID string) ([]string, error)
    ValidateCoupon(ctx context.Context, code string, orderAmount int64) (*Coupon, int64, error)
    SubmitReview(ctx context.Context, customerID string, req *ReviewRequest) error
    GetOrderTracking(ctx context.Context, customerID, orderID string) (*OrderTracking, error)
    GetNotifications(ctx context.Context, customerID string) ([]*NotificationItem, error)
    AddAddress(ctx context.Context, customerID, label, addressLine1, city, state, postalCode string) error
    ListAddresses(ctx context.Context, customerID string) ([]map[string]string, error)
}

type pgRepo struct {
    db  *pgxpool.Pool
    log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
    return &pgRepo{db: db, log: log}
}

func (r *pgRepo) CreateCustomer(ctx context.Context, c *Customer) (string, error) {
    var id string
    query := `INSERT INTO customers (name, email, phone, address) VALUES ($1,$2,$3,$4) RETURNING id`
    err := r.db.QueryRow(ctx, query, c.Name, c.Email, c.Phone, c.Address).Scan(&id)
    if err != nil {
        return "", err
    }
    return id, nil
}

func (r *pgRepo) GetCustomerByID(ctx context.Context, id string) (*Customer, error) {
    var c Customer
    query := `SELECT id, COALESCE(name, ''), COALESCE(email, ''), COALESCE(phone, ''), COALESCE(gender, ''), COALESCE(address, ''), created_at, updated_at FROM customers WHERE id=$1`
    row := r.db.QueryRow(ctx, query, id)
    err := row.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Gender, &c.Address, &c.CreatedAt, &c.UpdatedAt)
    if err != nil {
        return nil, err
    }
    return &c, nil
}

func (r *pgRepo) ListCustomers(ctx context.Context) ([]*Customer, error) {
    rows, err := r.db.Query(ctx, `SELECT id, name, email, phone, address, created_at, updated_at FROM customers`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []*Customer
    for rows.Next() {
        var c Customer
        if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Address, &c.CreatedAt, &c.UpdatedAt); err != nil {
            return nil, err
        }
        list = append(list, &c)
    }
    return list, rows.Err()
}

func (r *pgRepo) UpdateCustomer(ctx context.Context, c *Customer) error {
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    ct, err := tx.Exec(ctx, `UPDATE public.users SET full_name=$1, gender=$2, updated_at=NOW() WHERE id=$3 AND deleted_at IS NULL`, c.Name, c.Gender, c.ID)
    if err != nil {
        return err
    }
    if ct.RowsAffected() == 0 {
        return fmt.Errorf("user not found")
    }
    ct, err = tx.Exec(ctx, `UPDATE public.customers SET name=$1, gender=$2, updated_at=NOW() WHERE id=$3`, c.Name, c.Gender, c.ID)
    if err != nil {
        return err
    }
    if ct.RowsAffected() == 0 {
        return fmt.Errorf("customer profile not found")
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) AddAddress(ctx context.Context, customerID, label, addressLine1, city, state, postalCode string) error {
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    queryAddr := `INSERT INTO users.customer_addresses (customer_id, label, address_line1, city, state, postal_code, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
    if _, err = tx.Exec(ctx, queryAddr, customerID, label, addressLine1, city, state, postalCode); err != nil {
        return err
    }

    fullAddress := fmt.Sprintf("%s, %s, %s %s", addressLine1, city, state, postalCode)
    queryCust := `UPDATE public.customers SET address=$1, updated_at=NOW() WHERE id=$2`
    if _, err = tx.Exec(ctx, queryCust, fullAddress, customerID); err != nil {
        return err
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) ListAddresses(ctx context.Context, customerID string) ([]map[string]string, error) {
    query := `SELECT id, label, address_line1, city, state, postal_code FROM users.customer_addresses WHERE customer_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC`
    rows, err := r.db.Query(ctx, query, customerID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var list []map[string]string
    for rows.Next() {
        var id, label, line1, city, state, postalCode string
        if err := rows.Scan(&id, &label, &line1, &city, &state, &postalCode); err == nil {
            detail := fmt.Sprintf("%s, %s, %s %s", line1, city, state, postalCode)
            list = append(list, map[string]string{
                "id":     id,
                "name":   label,
                "detail": detail,
            })
        }
    }
    if len(list) == 0 {
        var custAddr string
        _ = r.db.QueryRow(ctx, `SELECT COALESCE(address, '') FROM public.customers WHERE id=$1`, customerID).Scan(&custAddr)
        if custAddr != "" {
            list = append(list, map[string]string{
                "id":     "primary",
                "name":   "Home",
                "detail": custAddr,
            })
        }
    }
    return list, nil
}

func (r *pgRepo) DeleteCustomer(ctx context.Context, id string) error {
    query := `DELETE FROM customers WHERE id=$1`
    _, err := r.db.Exec(ctx, query, id)
    return err
}

func (r *pgRepo) ListRestaurants(ctx context.Context, city, search string) ([]*Restaurant, error) {
    query := `
        SELECT m.id, m.name, COALESCE(array_to_string(m.tags, ', '), '') as cuisine,
               COALESCE((SELECT round(avg(rating),1)::text FROM merchants.restaurant_reviews WHERE merchant_id=m.id AND hidden_at IS NULL),'') as rating, (SELECT count(*)::text FROM merchants.restaurant_reviews WHERE merchant_id=m.id AND hidden_at IS NULL) as reviews, '' as time, '' as distance,
               '' as fee, '' as offer, '' as image, '' as accent
        FROM public.merchants m
        WHERE m.is_active = true AND m.approval_status='APPROVED' AND m.fee_payment_status='PAID' AND m.deleted_at IS NULL
    `
    query += ` AND ($1 = '' OR m.name ILIKE '%' || $1 || '%' OR EXISTS (
        SELECT 1 FROM catalog.items i
        WHERE i.merchant_id = m.id AND i.is_available = true AND i.deleted_at IS NULL
        AND i.name ILIKE '%' || $1 || '%'
    ))`
    rows, err := r.db.Query(ctx, query, search)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var result []*Restaurant
    for rows.Next() {
        var rest Restaurant
        err := rows.Scan(&rest.ID, &rest.Name, &rest.Cuisine, &rest.Rating, &rest.Reviews,
            &rest.Time, &rest.Distance, &rest.Fee, &rest.Offer, &rest.Image, &rest.Accent)
        if err != nil {
            return nil, err
        }

        // Fetch menu items for this restaurant
        menu, err := r.getMenuForMerchant(ctx, rest.ID)
        if err != nil {
            rest.Menu = []MenuItem{}
        } else {
            rest.Menu = menu
        }

        result = append(result, &rest)
    }

    return result, rows.Err()
}

func (r *pgRepo) GetRestaurantByID(ctx context.Context, id string) (*Restaurant, error) {
    query := `
        SELECT m.id, m.name, COALESCE(array_to_string(m.tags, ', '), '') as cuisine,
               COALESCE((SELECT round(avg(rating),1)::text FROM merchants.restaurant_reviews WHERE merchant_id=m.id AND hidden_at IS NULL),'') as rating, (SELECT count(*)::text FROM merchants.restaurant_reviews WHERE merchant_id=m.id AND hidden_at IS NULL) as reviews, '' as time, '' as distance,
               '' as fee, '' as offer, '' as image, '' as accent
        FROM public.merchants m
        WHERE m.id = $1 AND m.is_active = true AND m.approval_status='APPROVED' AND m.fee_payment_status='PAID' AND m.deleted_at IS NULL
    `
    var rest Restaurant
    err := r.db.QueryRow(ctx, query, id).Scan(
        &rest.ID, &rest.Name, &rest.Cuisine, &rest.Rating, &rest.Reviews,
        &rest.Time, &rest.Distance, &rest.Fee, &rest.Offer, &rest.Image, &rest.Accent,
    )
    if err != nil {
        return nil, err
    }

    menu, err := r.getMenuForMerchant(ctx, rest.ID)
    if err == nil {
        rest.Menu = menu
    } else {
        rest.Menu = []MenuItem{}
    }

    return &rest, nil
}

func (r *pgRepo) getMenuForMerchant(ctx context.Context, merchantID string) ([]MenuItem, error) {
    query := `
        SELECT i.id, i.name, i.description, (i.base_price / 100.0) as price,
               COALESCE(i.is_vegetarian, false) as is_veg,
               COALESCE(i.metadata->>'image_url', '') as image, '' as tag
        FROM catalog.items i
        WHERE i.merchant_id = $1 AND i.is_available = true AND i.deleted_at IS NULL
    `
    rows, err := r.db.Query(ctx, query, merchantID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var menu []MenuItem
    for rows.Next() {
        var item MenuItem
        err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Price, &item.IsVeg, &item.Image, &item.Tag)
        if err != nil {
            return nil, err
        }
        menu = append(menu, item)
    }
    return menu, rows.Err()
}

func (r *pgRepo) ToggleFavorite(ctx context.Context, customerID, merchantID string) (bool, error) {
    query := `
        INSERT INTO users.customer_favorites (customer_id, merchant_id)
        VALUES ($1, $2)
        ON CONFLICT (customer_id, merchant_id) DO NOTHING
    `
    tag, err := r.db.Exec(ctx, query, customerID, merchantID)
    if err != nil {
        return false, err
    }

    if tag.RowsAffected() == 0 {
        // Was already favorite, remove it
        deleteQuery := `DELETE FROM users.customer_favorites WHERE customer_id = $1 AND merchant_id = $2`
        _, err := r.db.Exec(ctx, deleteQuery, customerID, merchantID)
        return false, err
    }

    return true, nil
}

func (r *pgRepo) GetFavorites(ctx context.Context, customerID string) ([]string, error) {
    query := `SELECT merchant_id FROM users.customer_favorites WHERE customer_id = $1`
    rows, err := r.db.Query(ctx, query, customerID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var favorites []string
    for rows.Next() {
        var merchantID string
        if err := rows.Scan(&merchantID); err != nil {
            return nil, err
        }
        favorites = append(favorites, merchantID)
    }
    return favorites, rows.Err()
}

func (r *pgRepo) ValidateCoupon(ctx context.Context, code string, orderAmount int64) (*Coupon, int64, error) {
    query := `
        SELECT id, code, description, discount_type, discount_value, min_order_amount, is_active
        FROM orders.coupons
        WHERE UPPER(code) = UPPER($1) AND is_active = true
    `
    var c Coupon
    err := r.db.QueryRow(ctx, query, code).Scan(
        &c.ID, &c.Code, &c.Description, &c.DiscountType, &c.DiscountValue, &c.MinOrderAmount, &c.IsActive,
    )
    if err != nil {
        return nil, 0, fmt.Errorf("coupon not found or invalid")
    }

    if orderAmount < c.MinOrderAmount {
        return nil, 0, fmt.Errorf("order amount below minimum requirement of ₹%d", c.MinOrderAmount)
    }

    var discount int64
    if c.DiscountType == "PERCENTAGE" {
        discount = (orderAmount * c.DiscountValue) / 100
    } else {
        discount = c.DiscountValue
    }

    return &c, discount, nil
}

func (r *pgRepo) SubmitReview(ctx context.Context, customerID string, req *ReviewRequest) error {
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    // Get merchant_id from order
    var merchantID string
    query := `SELECT merchant_id FROM public.orders WHERE id = $1 AND customer_id = $2 AND status='DELIVERED' FOR UPDATE`
    err = tx.QueryRow(ctx, query, req.OrderID, customerID).Scan(&merchantID)
    if err != nil {
        return fmt.Errorf("order not found: %w", err)
    }

    insertQuery := `
        INSERT INTO merchants.restaurant_reviews (order_id, customer_id, merchant_id, rating, comment)
        VALUES ($1, $2, $3, $4, $5)
    `
    _, err = tx.Exec(ctx, insertQuery, req.OrderID, customerID, merchantID, req.Rating, req.Comment)
    if err != nil {
        return fmt.Errorf("review already submitted or invalid")
    }
    if req.DeliveryRating > 0 {
        tag, err := tx.Exec(ctx, `INSERT INTO delivery.partner_ratings(order_id,partner_id,customer_id,rating) SELECT order_id,partner_id,$2,$3 FROM delivery.delivery_assignments WHERE order_id=$1 AND status='DELIVERED'`, req.OrderID, customerID, req.DeliveryRating)
        if err != nil {
            return err
        }
        if tag.RowsAffected() != 1 {
            return fmt.Errorf("completed delivery required for partner rating")
        }
    }
    return tx.Commit(ctx)
}

func (r *pgRepo) GetOrderTracking(ctx context.Context, customerID, orderID string) (*OrderTracking, error) {
    query := `
        SELECT o.id, o.status, m.name, mb.address,
               COALESCE(dp.name, 'Assigning rider...'), COALESCE(dp.phone, ''),
               COALESCE(ll.latitude, 0), COALESCE(ll.longitude, 0),
               0 as estimated_minutes, o.updated_at
        FROM public.orders o
        JOIN public.merchants m ON o.merchant_id = m.id
        LEFT JOIN merchants.merchant_branches mb ON m.id = mb.merchant_id
        LEFT JOIN delivery.delivery_assignments da ON o.id = da.order_id
        LEFT JOIN delivery.delivery_partners dp ON da.partner_id = dp.id
        LEFT JOIN delivery.live_locations ll ON dp.id = ll.partner_id
        WHERE o.customer_id = $1 AND o.id = $2
    `
    var track OrderTracking
    err := r.db.QueryRow(ctx, query, customerID, orderID).Scan(
        &track.OrderID, &track.Status, &track.MerchantName, &track.MerchantAddress,
        &track.RiderName, &track.RiderPhone, &track.RiderLatitude, &track.RiderLongitude,
        &track.EstimatedMinutes, &track.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &track, nil
}

func (r *pgRepo) GetNotifications(ctx context.Context, customerID string) ([]*NotificationItem, error) {
    rows, err := r.db.Query(ctx, `SELECT id,COALESCE(user_id::text,''),channel,template_key,payload,status,created_at FROM notifications.notification_logs WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, customerID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    list := []*NotificationItem{}
    for rows.Next() {
        var item NotificationItem
        var payload map[string]interface{}
        if err = rows.Scan(&item.ID, &item.UserID, &item.Channel, &item.TemplateKey, &payload, &item.Status, &item.CreatedAt); err != nil {
            return nil, err
        }
        item.Title, _ = payload["title"].(string)
        item.Message, _ = payload["message"].(string)
        item.Read = item.Status == "READ"
        list = append(list, &item)
    }
    return list, rows.Err()
}

package restaurant

import "context"

func (r *pgRepo) ListMenuItems(ctx context.Context, merchant string) ([]MenuItemInput, error) {
    rows, err := r.db.Query(ctx, `SELECT id,merchant_id,category_id,name,description,COALESCE(is_vegetarian,false),base_price/100.0,is_available,COALESCE(metadata->>'image_url','') FROM catalog.items WHERE merchant_id=$1 AND deleted_at IS NULL ORDER BY name`, merchant)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    items := []MenuItemInput{}
    for rows.Next() {
        var i MenuItemInput
        if err = rows.Scan(&i.ID, &i.MerchantID, &i.CategoryID, &i.Name, &i.Description, &i.IsVeg, &i.Price, &i.IsAvailable, &i.ImageURL); err != nil {
            return nil, err
        }
        items = append(items, i)
    }
    return items, rows.Err()
}

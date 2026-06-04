# SwiftKart API

Base path: `/api/v1`

## Health

- `GET /healthz`
- `GET /readyz`

## Auth

- `POST /api/v1/auth/signup`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/auth/me`

## Customer

- `POST /api/v1/customer/create`
- `GET /api/v1/customer/:id`
- `GET /api/v1/customer`
- `PUT /api/v1/customer/:id`
- `DELETE /api/v1/customer/:id`

## Merchant

- `POST /api/v1/merchant/create`
- `GET /api/v1/merchant/:id`
- `GET /api/v1/merchant`
- `PUT /api/v1/merchant/:id`
- `DELETE /api/v1/merchant/:id`

## Catalog

- `POST /api/v1/catalog/categories`
- `POST /api/v1/catalog/items`
- `GET /api/v1/catalog/items?q=coffee&limit=25`
- `GET /api/v1/catalog/items/:id`
- `GET /api/v1/catalog/merchants/:merchant_id/items`
- `PUT /api/v1/catalog/items/:id`

Example item:

```json
{
  "merchant_id": "merchant-uuid",
  "category_id": "category-uuid",
  "name": "Paneer Roll",
  "description": "Generic catalog item, not a food-only menu entry",
  "base_price": 14900,
  "currency": "INR",
  "is_vegetarian": true,
  "inventory_tracked": false
}
```

## Orders

- `POST /api/v1/order/create`
- `GET /api/v1/order/:id`
- `GET /api/v1/order`
- `PUT /api/v1/order/:id/status`
- `DELETE /api/v1/order/:id`

Order statuses: `CREATED`, `PAYMENT_PENDING`, `CONFIRMED`, `PREPARING`, `READY_FOR_PICKUP`, `PICKED_UP`, `OUT_FOR_DELIVERY`, `DELIVERED`, `CANCELLED`, `REFUNDED`.

## Delivery

- `POST /api/v1/delivery/partners/:id/online`
- `POST /api/v1/delivery/partners/:id/offline`
- `POST /api/v1/delivery/locations`
- `POST /api/v1/delivery/assignments`

## Payments

- `POST /api/v1/payments/intents`

## Development

```bash
make docker-up
make migrate
make run
```

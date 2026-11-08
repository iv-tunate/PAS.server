-- name: CreateOrder :one
INSERT INTO orders (kind, item_id, item_title, buyer_name, buyer_email, buyer_phone,
                    gateway, reference, amount, currency, status, download_token)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;

-- name: GetOrderByReference :one
SELECT * FROM orders WHERE reference = $1;

-- name: GetPaidOrderByDownloadToken :one
SELECT * FROM orders WHERE download_token = $1 AND status = 'success';

-- name: SetOrderGatewayRef :exec
UPDATE orders SET gateway_ref = $2, updated_at = now() WHERE id = $1;

-- name: MarkOrderSuccess :one
-- Only the first caller sees a row back. The payment webhook and the
-- buyer's return page can both confirm the same order, and this makes
-- sure only one of them triggers the delivery email.
UPDATE orders SET status = 'success', updated_at = now()
WHERE reference = $1 AND status <> 'success'
RETURNING *;

-- name: MarkOrderFailed :exec
UPDATE orders SET status = 'failed', updated_at = now()
WHERE reference = $1 AND status = 'pending';

-- name: TouchOrder :exec
UPDATE orders SET updated_at = now() WHERE id = $1;

-- name: MarkOrderDelivered :exec
UPDATE orders SET delivered_at = now(), updated_at = now() WHERE id = $1;

-- name: BumpDeliveryAttempt :exec
UPDATE orders SET delivery_attempts = delivery_attempts + 1, updated_at = now() WHERE id = $1;

-- name: IncrementDownloadCount :exec
UPDATE orders SET download_count = download_count + 1 WHERE id = $1;

-- name: ListOrdersAwaitingDelivery :many
-- Paid orders whose email has not gone out: the queue was full, Brevo was
-- down, or the server restarted. Orders touched in the last two minutes are
-- skipped so a delivery already in flight is not doubled up.
SELECT * FROM orders
WHERE status = 'success'
  AND delivered_at IS NULL
  AND delivery_attempts < 5
  AND created_at > now() - interval '3 days'
  AND updated_at < now() - interval '2 minutes'
ORDER BY created_at
LIMIT 50;

-- name: ListStalePendingOrders :many
-- Paid-for-but-never-confirmed orders (webhook missed, tab closed) get
-- re-checked with the gateway for a day after they were started.
SELECT * FROM orders
WHERE status = 'pending'
  AND gateway <> 'free'
  AND created_at > now() - interval '1 day'
  AND updated_at < now() - interval '10 minutes'
ORDER BY created_at
LIMIT 50;

-- name: ListOrders :many
-- Admin: newest first. An empty status means "all".
SELECT * FROM orders
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER BY created_at DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountOrders :one
SELECT count(*) FROM orders
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text);

-- name: CountRecentOrdersByEmail :one
-- Used to stop one address being used to flood someone's inbox.
SELECT count(*) FROM orders
WHERE lower(buyer_email) = lower($1) AND created_at > now() - interval '1 hour';

-- name: SumRevenueByCurrency :many
SELECT currency, COALESCE(sum(amount), 0)::bigint AS total
FROM orders
WHERE status = 'success' AND gateway <> 'free'
GROUP BY currency;

-- name: CountOrdersByStatus :one
SELECT count(*) FROM orders WHERE status = $1;

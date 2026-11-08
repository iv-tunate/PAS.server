-- name: ListSiteContent :many
SELECT key, value, updated_at FROM site_content ORDER BY key;

-- name: UpsertSiteContent :one
INSERT INTO site_content (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
RETURNING key, value, updated_at;

-- name: DeleteSiteContent :exec
DELETE FROM site_content WHERE key = $1;

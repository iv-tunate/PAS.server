-- name: ListPublishedMessages :many
SELECT * FROM messages
WHERE is_published = TRUE
ORDER BY preached_on DESC NULLS LAST, created_at DESC
LIMIT 200;

-- name: ListAllMessages :many
SELECT * FROM messages
ORDER BY preached_on DESC NULLS LAST, created_at DESC;

-- name: CreateMessage :one
INSERT INTO messages (title, description, youtube_id, preached_on, is_published)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateMessage :one
UPDATE messages
SET title = $2, description = $3, youtube_id = $4, preached_on = $5, is_published = $6, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMessage :exec
DELETE FROM messages WHERE id = $1;

-- name: CountMessages :one
SELECT count(*) FROM messages;

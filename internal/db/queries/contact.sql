-- name: CreateContactMessage :one
INSERT INTO contact_messages (topic, name, email, phone, preferred_time, body)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListContactMessages :many
SELECT * FROM contact_messages
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: SetContactMessageRead :exec
UPDATE contact_messages SET is_read = $2 WHERE id = $1;

-- name: DeleteContactMessage :exec
DELETE FROM contact_messages WHERE id = $1;

-- name: CountUnreadContactMessages :one
SELECT count(*) FROM contact_messages WHERE is_read = FALSE;

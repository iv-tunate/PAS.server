-- name: ListPublishedEvents :many
SELECT * FROM events
WHERE is_published = TRUE
ORDER BY position, created_at;

-- name: ListAllEvents :many
SELECT * FROM events
ORDER BY position, created_at;

-- name: CreateEvent :one
INSERT INTO events (name, description, month, day_start, day_end, flyer_url, position, is_published)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateEvent :one
UPDATE events
SET name = $2, description = $3, month = $4, day_start = $5, day_end = $6,
    flyer_url = $7, position = $8, is_published = $9, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetEventPosition :exec
UPDATE events SET position = $2, updated_at = now() WHERE id = $1;

-- name: DeleteEvent :exec
DELETE FROM events WHERE id = $1;

-- name: NextEventPosition :one
SELECT COALESCE(max(position), 0)::int + 1 FROM events;

-- name: CountEvents :one
SELECT count(*) FROM events;

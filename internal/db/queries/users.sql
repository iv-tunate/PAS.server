-- Accounts exist for administrators only.

-- name: CreateUser :one
INSERT INTO users (full_name, email, password_hash, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower($1);

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: ListAdmins :many
SELECT * FROM users WHERE role = 'admin' ORDER BY created_at;

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE role = 'admin';

-- name: DeleteAdmin :exec
DELETE FROM users WHERE id = $1 AND role = 'admin';

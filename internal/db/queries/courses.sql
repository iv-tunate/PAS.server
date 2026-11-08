-- name: ListPublishedCourses :many
-- Public: what the Gateway Teachings section fetches.
SELECT * FROM courses
WHERE is_published = TRUE AND file_url <> ''
ORDER BY created_at DESC;

-- name: ListAllCourses :many
-- Admin: includes unpublished/draft courses.
SELECT * FROM courses
ORDER BY created_at DESC;

-- name: GetCourseBySlug :one
SELECT * FROM courses WHERE slug = $1;

-- name: GetCourseByID :one
SELECT * FROM courses WHERE id = $1;

-- name: CreateCourse :one
INSERT INTO courses (title, slug, summary, thumbnail_url, media_kind, file_url, is_free, price_kobo, price_usd_cents, is_published)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateCourse :one
UPDATE courses
SET title = $2, summary = $3, thumbnail_url = $4, media_kind = $5, file_url = $6,
    is_free = $7, price_kobo = $8, price_usd_cents = $9, is_published = $10, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteCourse :exec
DELETE FROM courses WHERE id = $1;

-- name: ListChaptersByCourse :many
SELECT * FROM course_chapters
WHERE course_id = $1
ORDER BY position ASC;

-- name: CreateChapter :one
INSERT INTO course_chapters (course_id, title, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteChaptersByCourse :exec
DELETE FROM course_chapters WHERE course_id = $1;

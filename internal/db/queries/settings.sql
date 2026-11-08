-- name: GetSiteSettings :one
SELECT * FROM site_settings WHERE id = TRUE;

-- name: UpdateSiteSettings :one
UPDATE site_settings SET
    pastor_name = $1,
    pastor_bio = $2,
    pastor_photo_url = $3,
    campuses_count = $4,
    years_leadership = $5,
    continents_reached = $6,
    books_url = $7,
    website_url = $8,
    contact_email = $9,
    hero_video_url = $10,
    updated_at = now()
WHERE id = TRUE
RETURNING *;

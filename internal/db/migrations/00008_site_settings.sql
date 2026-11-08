-- +goose Up
CREATE TABLE
    site_settings (
        id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id = TRUE), -- enforces exactly one row
        pastor_name TEXT NOT NULL DEFAULT 'Akintola Samuel',
        pastor_bio TEXT NOT NULL DEFAULT '',
        pastor_photo_url TEXT NOT NULL DEFAULT '',
        campuses_count TEXT NOT NULL DEFAULT '',
        years_leadership TEXT NOT NULL DEFAULT '',
        continents_reached TEXT NOT NULL DEFAULT '',
        books_url TEXT NOT NULL DEFAULT '',
        website_url TEXT NOT NULL DEFAULT '',
        contact_email TEXT NOT NULL DEFAULT '',
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

INSERT INTO
    site_settings (id)
VALUES
    (TRUE);

ALTER TABLE courses
ADD COLUMN thumbnail_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE courses
DROP COLUMN IF EXISTS thumbnail_url;

DROP TABLE IF EXISTS site_settings;
-- +goose Up
-- Editable website copy and photos. One row per piece of content, keyed
-- by a name like "about.bio" or "photo.3". A missing row means "show
-- the built-in placeholder", so an admin can always reset a field by
-- deleting its row.
CREATE TABLE
    site_content (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

-- +goose Down
DROP TABLE IF EXISTS site_content;

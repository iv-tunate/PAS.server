-- +goose Up
ALTER TABLE site_settings ADD COLUMN hero_video_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE site_settings DROP COLUMN IF EXISTS hero_video_url;

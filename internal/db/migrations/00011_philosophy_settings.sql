-- +goose Up
ALTER TABLE site_settings
ADD COLUMN philosophy_1_title TEXT NOT NULL DEFAULT 'Eternal truths';

ALTER TABLE site_settings
ADD COLUMN philosophy_1_body TEXT NOT NULL DEFAULT 'We focus on timeless biblical principles that build a lasting foundation for your life.';

ALTER TABLE site_settings
ADD COLUMN philosophy_2_title TEXT NOT NULL DEFAULT 'A quiet mind';

ALTER TABLE site_settings
ADD COLUMN philosophy_2_body TEXT NOT NULL DEFAULT 'We help you protect your time and attention from the endless pull of social media.';

ALTER TABLE site_settings
ADD COLUMN philosophy_3_title TEXT NOT NULL DEFAULT 'Practical faith';

ALTER TABLE site_settings
ADD COLUMN philosophy_3_body TEXT NOT NULL DEFAULT 'We turn deep teaching into simple, daily actions that shape your choices.';

-- +goose Down
ALTER TABLE site_settings
DROP COLUMN IF EXISTS philosophy_1_title;

ALTER TABLE site_settings
DROP COLUMN IF EXISTS philosophy_1_body;

ALTER TABLE site_settings
DROP COLUMN IF EXISTS philosophy_2_title;

ALTER TABLE site_settings
DROP COLUMN IF EXISTS philosophy_2_body;

ALTER TABLE site_settings
DROP COLUMN IF EXISTS philosophy_3_title;

ALTER TABLE site_settings
DROP COLUMN IF EXISTS philosophy_3_body;
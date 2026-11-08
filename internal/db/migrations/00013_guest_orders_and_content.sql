-- +goose Up

-- ---------------------------------------------------------------------
-- Buyers no longer have accounts. A purchase (or a free download) is just
-- an order that remembers who asked, so the admin can reach out and the
-- download link can be emailed. These three tables are replaced by `orders`.
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS enrollments;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS book_purchases;

ALTER TABLE books
    ADD COLUMN is_free BOOLEAN NOT NULL DEFAULT FALSE;

-- A course is now one downloadable thing (video, audio or a file bundle).
-- The chapter list stays as an optional curriculum shown before buying.
ALTER TABLE courses
    ADD COLUMN is_free BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN price_usd_cents BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN media_kind TEXT NOT NULL DEFAULT 'video' CHECK (media_kind IN ('video', 'audio', 'file')),
    ADD COLUMN file_url TEXT NOT NULL DEFAULT '';

CREATE TABLE
    orders (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        kind TEXT NOT NULL CHECK (kind IN ('book', 'course')),
        item_id UUID NOT NULL,
        item_title TEXT NOT NULL,
        buyer_name TEXT NOT NULL,
        buyer_email TEXT NOT NULL,
        buyer_phone TEXT NOT NULL,
        gateway TEXT NOT NULL CHECK (gateway IN ('free', 'paystack', 'flutterwave', 'stripe')),
        reference TEXT NOT NULL UNIQUE,
        gateway_ref TEXT NOT NULL DEFAULT '',
        amount BIGINT NOT NULL DEFAULT 0,
        currency TEXT NOT NULL DEFAULT 'NGN' CHECK (currency IN ('NGN', 'USD')),
        status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
        download_token TEXT NOT NULL UNIQUE,
        download_count INT NOT NULL DEFAULT 0,
        delivery_attempts INT NOT NULL DEFAULT 0,
        delivered_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE INDEX idx_orders_status ON orders (status);

CREATE INDEX idx_orders_created_at ON orders (created_at DESC);

CREATE INDEX idx_orders_buyer_email ON orders (lower(buyer_email));

-- ---------------------------------------------------------------------
-- Messages: a YouTube link with a title. Only the 11-character video id
-- is stored; the thumbnail and the player are built from it.
-- ---------------------------------------------------------------------
CREATE TABLE
    messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        title TEXT NOT NULL,
        description TEXT NOT NULL DEFAULT '',
        youtube_id TEXT NOT NULL,
        preached_on DATE,
        is_published BOOLEAN NOT NULL DEFAULT TRUE,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE INDEX idx_messages_preached_on ON messages (preached_on DESC NULLS LAST, created_at DESC);

-- ---------------------------------------------------------------------
-- Events. month / day_start / day_end use 0 for "not specified" so the
-- card can show just a name and a description when there is no date.
-- ---------------------------------------------------------------------
CREATE TABLE
    events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        name TEXT NOT NULL,
        description TEXT NOT NULL DEFAULT '',
        month SMALLINT NOT NULL DEFAULT 0 CHECK (month BETWEEN 0 AND 12),
        day_start SMALLINT NOT NULL DEFAULT 0 CHECK (day_start BETWEEN 0 AND 31),
        day_end SMALLINT NOT NULL DEFAULT 0 CHECK (day_end BETWEEN 0 AND 31),
        flyer_url TEXT NOT NULL DEFAULT '',
        position INT NOT NULL DEFAULT 0,
        is_published BOOLEAN NOT NULL DEFAULT TRUE,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        CHECK (month > 0 OR (day_start = 0 AND day_end = 0)),
        CHECK (day_end = 0 OR day_end >= day_start)
    );

CREATE INDEX idx_events_position ON events (position, created_at);

INSERT INTO
    events (name, description, month, day_start, day_end, flyer_url, position)
VALUES
    (
        'Breakthrough Festival',
        'Twelve days at the very start of the year given to prayer, the Word and faith for what is ahead.',
        1, 1, 12, '', 1
    ),
    (
        'Code Red Revival',
        'Six days in July set apart for revival: worship, teaching and prayer, together as a church family.',
        7, 1, 6, '', 2
    ),
    (
        'Upsurge',
        'A gathering on sudden growth, spiritual increase and advancement. This is your season to break forth, break through and move higher.',
        10, 2, 4,
        'https://gatewaynation.org/wp-content/uploads/2026/09/WhatsApp-Image-2026-09-26-at-09.47.32.jpeg',
        3
    ),
    (
        'Love Lounge',
        'Honest conversations about love, relationships and marriage, led by Pastor Akintola Samuel.',
        0, 0, 0,
        'https://gatewaynation.org/wp-content/uploads/2025/01/LOVE-LOUNGE.jpg',
        4
    );

-- ---------------------------------------------------------------------
-- Messages people send to the pastor through the contact form.
-- ---------------------------------------------------------------------
CREATE TABLE
    contact_messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        topic TEXT NOT NULL DEFAULT 'message' CHECK (topic IN ('message', 'appointment', 'prayer')),
        name TEXT NOT NULL,
        email TEXT NOT NULL,
        phone TEXT NOT NULL DEFAULT '',
        preferred_time TEXT NOT NULL DEFAULT '',
        body TEXT NOT NULL,
        is_read BOOLEAN NOT NULL DEFAULT FALSE,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE INDEX idx_contact_messages_created_at ON contact_messages (created_at DESC);

-- ---------------------------------------------------------------------
-- Newsletter: every subscriber gets a private unsubscribe token, and each
-- broadcast the admin sends is recorded so progress can be watched.
-- ---------------------------------------------------------------------
ALTER TABLE subscribers
    ADD COLUMN unsubscribe_token TEXT NOT NULL DEFAULT encode(gen_random_bytes(16), 'hex');

CREATE UNIQUE INDEX idx_subscribers_unsubscribe_token ON subscribers (unsubscribe_token);

CREATE TABLE
    newsletter_campaigns (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        subject TEXT NOT NULL,
        body TEXT NOT NULL,
        status TEXT NOT NULL DEFAULT 'sending' CHECK (status IN ('sending', 'done', 'interrupted')),
        total INT NOT NULL DEFAULT 0,
        sent INT NOT NULL DEFAULT 0,
        failed INT NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        finished_at TIMESTAMPTZ
    );

-- +goose Down
-- The old per-user tables (enrollments, payments, book_purchases) are not
-- recreated: they belonged to the account-based flow this migration retires.
DROP TABLE IF EXISTS newsletter_campaigns;

DROP INDEX IF EXISTS idx_subscribers_unsubscribe_token;

ALTER TABLE subscribers
DROP COLUMN IF EXISTS unsubscribe_token;

DROP TABLE IF EXISTS contact_messages;

DROP TABLE IF EXISTS events;

DROP TABLE IF EXISTS messages;

DROP TABLE IF EXISTS orders;

ALTER TABLE courses
DROP COLUMN IF EXISTS file_url,
DROP COLUMN IF EXISTS media_kind,
DROP COLUMN IF EXISTS price_usd_cents,
DROP COLUMN IF EXISTS is_free;

ALTER TABLE books
DROP COLUMN IF EXISTS is_free;

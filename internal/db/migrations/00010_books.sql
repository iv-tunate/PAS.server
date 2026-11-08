-- +goose Up
CREATE TABLE
    books (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        title TEXT NOT NULL,
        author TEXT NOT NULL DEFAULT 'Akintola Samuel',
        description TEXT NOT NULL DEFAULT '',
        cover_image_url TEXT NOT NULL DEFAULT '',
        file_url TEXT NOT NULL DEFAULT '',
        price_kobo BIGINT NOT NULL DEFAULT 0,
        price_usd_cents BIGINT NOT NULL DEFAULT 0,
        is_published BOOLEAN NOT NULL DEFAULT TRUE,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE INDEX idx_books_is_published ON books (is_published);

CREATE TABLE
    book_purchases (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
        book_id UUID NOT NULL REFERENCES books (id) ON DELETE CASCADE,
        gateway TEXT NOT NULL CHECK (gateway IN ('paystack', 'flutterwave', 'stripe')),
        reference TEXT NOT NULL UNIQUE,
        amount BIGINT NOT NULL,
        currency TEXT NOT NULL CHECK (currency IN ('NGN', 'USD')),
        status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
        created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE INDEX idx_book_purchases_user_id ON book_purchases (user_id);

CREATE INDEX idx_book_purchases_reference ON book_purchases (reference);

CREATE INDEX idx_book_purchases_status ON book_purchases (status);

-- +goose Down
DROP TABLE IF EXISTS book_purchases;

DROP TABLE IF EXISTS books;
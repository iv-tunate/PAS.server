-- +goose Up
CREATE TABLE
    courses (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        title TEXT NOT NULL,
        slug TEXT NOT NULL UNIQUE,
        summary TEXT NOT NULL DEFAULT '',
        price_kobo BIGINT NOT NULL DEFAULT 0,
        is_published BOOLEAN NOT NULL DEFAULT FALSE,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE TABLE
    course_chapters (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
        course_id UUID NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
        title TEXT NOT NULL,
        position INT NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE INDEX idx_course_chapters_course_id ON course_chapters (course_id);

CREATE INDEX idx_courses_is_published ON courses (is_published);

-- +goose Down
DROP TABLE IF EXISTS course_chapters;

DROP TABLE IF EXISTS courses;
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
)

type CourseService struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewCourseService(pool *pgxpool.Pool) *CourseService {
	return &CourseService{pool: pool, queries: sqlc.New(pool)}
}

// PublicCourse is what visitors get. Like books, the file address stays
// private until an order is placed.
type PublicCourse struct {
	ID            uuid.UUID       `json:"id"`
	Title         string          `json:"title"`
	Slug          string          `json:"slug"`
	Summary       string          `json:"summary"`
	ThumbnailURL  string          `json:"thumbnail_url"`
	MediaKind     string          `json:"media_kind"`
	IsFree        bool            `json:"is_free"`
	PriceKobo     int64           `json:"price_kobo"`
	PriceUSDCents int64           `json:"price_usd_cents"`
	Chapters      []PublicChapter `json:"chapters"`
}

type PublicChapter struct {
	Title    string `json:"title"`
	Position int32  `json:"position"`
}

// AdminCourse is the full record plus its chapters, for the dashboard.
type AdminCourse struct {
	sqlc.Course
	Chapters []PublicChapter `json:"chapters"`
}

func (s *CourseService) chaptersFor(ctx context.Context, courseID uuid.UUID) ([]PublicChapter, error) {
	rows, err := s.queries.ListChaptersByCourse(ctx, courseID)
	if err != nil {
		return nil, err
	}
	out := make([]PublicChapter, 0, len(rows))
	for _, ch := range rows {
		out = append(out, PublicChapter{Title: ch.Title, Position: ch.Position})
	}
	return out, nil
}

func (s *CourseService) toPublic(ctx context.Context, c sqlc.Course) (PublicCourse, error) {
	chapters, err := s.chaptersFor(ctx, c.ID)
	if err != nil {
		return PublicCourse{}, err
	}
	return PublicCourse{
		ID: c.ID, Title: c.Title, Slug: c.Slug, Summary: c.Summary,
		ThumbnailURL: c.ThumbnailUrl, MediaKind: c.MediaKind, IsFree: c.IsFree,
		PriceKobo: c.PriceKobo, PriceUSDCents: c.PriceUsdCents, Chapters: chapters,
	}, nil
}

func (s *CourseService) ListPublished(ctx context.Context) ([]PublicCourse, error) {
	rows, err := s.queries.ListPublishedCourses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PublicCourse, 0, len(rows))
	for _, c := range rows {
		pc, err := s.toPublic(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return out, nil
}

func (s *CourseService) GetBySlug(ctx context.Context, slug string) (PublicCourse, error) {
	c, err := s.queries.GetCourseBySlug(ctx, slug)
	if err != nil {
		return PublicCourse{}, err
	}
	if !c.IsPublished || c.FileUrl == "" {
		return PublicCourse{}, ErrItemNotFound
	}
	return s.toPublic(ctx, c)
}

func (s *CourseService) ListAllForAdmin(ctx context.Context) ([]AdminCourse, error) {
	rows, err := s.queries.ListAllCourses(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminCourse, 0, len(rows))
	for _, c := range rows {
		chapters, err := s.chaptersFor(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, AdminCourse{Course: c, Chapters: chapters})
	}
	return out, nil
}

type CourseInput struct {
	Title         string
	Summary       string
	ThumbnailURL  string
	MediaKind     string // video | audio | file
	FileURL       string
	IsFree        bool
	PriceKobo     int64
	PriceUSDCents int64
	IsPublished   bool
	Chapters      []string // ordered curriculum titles (optional)
}

// Create saves the course and its chapters together or not at all.
func (s *CourseService) Create(ctx context.Context, in CourseInput) (AdminCourse, error) {
	base := slugify(in.Title)

	// Two courses can share a title. The web address (slug) must be unique, so
	// if it is taken we try again with a short random ending.
	for attempt := 0; attempt < 5; attempt++ {
		slug := base
		if attempt > 0 {
			slug = base + "-" + shortSuffix()
		}

		var created sqlc.Course
		err := s.inTx(ctx, func(q *sqlc.Queries) error {
			c, err := q.CreateCourse(ctx, sqlc.CreateCourseParams{
				Title: in.Title, Slug: slug, Summary: in.Summary, ThumbnailUrl: in.ThumbnailURL,
				MediaKind: in.MediaKind, FileUrl: in.FileURL, IsFree: in.IsFree,
				PriceKobo: in.PriceKobo, PriceUsdCents: in.PriceUSDCents, IsPublished: in.IsPublished,
			})
			if err != nil {
				return err
			}
			created = c
			return s.writeChapters(ctx, q, c.ID, in.Chapters)
		})
		if isUniqueViolation(err) {
			continue
		}
		if err != nil {
			return AdminCourse{}, err
		}
		chapters, err := s.chaptersFor(ctx, created.ID)
		return AdminCourse{Course: created, Chapters: chapters}, err
	}
	return AdminCourse{}, errors.New("could not find a free web address for this course")
}

// Update replaces the course details and its chapter list.
func (s *CourseService) Update(ctx context.Context, id uuid.UUID, in CourseInput) (AdminCourse, error) {
	var updated sqlc.Course
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		c, err := q.UpdateCourse(ctx, sqlc.UpdateCourseParams{
			ID: id, Title: in.Title, Summary: in.Summary, ThumbnailUrl: in.ThumbnailURL,
			MediaKind: in.MediaKind, FileUrl: in.FileURL, IsFree: in.IsFree,
			PriceKobo: in.PriceKobo, PriceUsdCents: in.PriceUSDCents, IsPublished: in.IsPublished,
		})
		if err != nil {
			return err
		}
		updated = c
		if err := q.DeleteChaptersByCourse(ctx, id); err != nil {
			return err
		}
		return s.writeChapters(ctx, q, id, in.Chapters)
	})
	if err != nil {
		return AdminCourse{}, err
	}
	chapters, err := s.chaptersFor(ctx, id)
	return AdminCourse{Course: updated, Chapters: chapters}, err
}

func (s *CourseService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteCourse(ctx, id)
}

func (s *CourseService) writeChapters(ctx context.Context, q *sqlc.Queries, courseID uuid.UUID, titles []string) error {
	position := int32(0)
	for _, title := range titles {
		title = oneLine(title)
		if title == "" {
			continue
		}
		if _, err := q.CreateChapter(ctx, sqlc.CreateChapterParams{CourseID: courseID, Title: title, Position: position}); err != nil {
			return err
		}
		position++
	}
	return nil
}

func (s *CourseService) inTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after a successful commit

	if err := fn(s.queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func shortSuffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "course"
	}
	return slug
}

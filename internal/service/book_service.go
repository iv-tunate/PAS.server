package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
)

type BookService struct {
	queries *sqlc.Queries
}

func NewBookService(pool *pgxpool.Pool) *BookService {
	return &BookService{queries: sqlc.New(pool)}
}

// PublicBook is what visitors get. The file address is left out: it is only
// ever revealed through a download link emailed after an order.
type PublicBook struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Description   string    `json:"description"`
	CoverImageURL string    `json:"cover_image_url"`
	IsFree        bool      `json:"is_free"`
	PriceKobo     int64     `json:"price_kobo"`
	PriceUSDCents int64     `json:"price_usd_cents"`
}

func (s *BookService) ListPublished(ctx context.Context) ([]PublicBook, error) {
	rows, err := s.queries.ListPublishedBooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PublicBook, 0, len(rows))
	for _, b := range rows {
		out = append(out, PublicBook{
			ID: b.ID, Title: b.Title, Author: b.Author, Description: b.Description,
			CoverImageURL: b.CoverImageUrl, IsFree: b.IsFree,
			PriceKobo: b.PriceKobo, PriceUSDCents: b.PriceUsdCents,
		})
	}
	return out, nil
}

// ListAllForAdmin returns everything, including the file address and drafts.
func (s *BookService) ListAllForAdmin(ctx context.Context) ([]sqlc.Book, error) {
	return s.queries.ListAllBooks(ctx)
}

type BookInput struct {
	Title         string
	Author        string
	Description   string
	CoverImageURL string
	FileURL       string
	IsFree        bool
	PriceKobo     int64
	PriceUSDCents int64
	IsPublished   bool
}

func (s *BookService) Create(ctx context.Context, in BookInput) (sqlc.Book, error) {
	return s.queries.CreateBook(ctx, sqlc.CreateBookParams{
		Title: in.Title, Author: in.Author, Description: in.Description,
		CoverImageUrl: in.CoverImageURL, FileUrl: in.FileURL, IsFree: in.IsFree,
		PriceKobo: in.PriceKobo, PriceUsdCents: in.PriceUSDCents, IsPublished: in.IsPublished,
	})
}

func (s *BookService) Update(ctx context.Context, id uuid.UUID, in BookInput) (sqlc.Book, error) {
	return s.queries.UpdateBook(ctx, sqlc.UpdateBookParams{
		ID: id, Title: in.Title, Author: in.Author, Description: in.Description,
		CoverImageUrl: in.CoverImageURL, FileUrl: in.FileURL, IsFree: in.IsFree,
		PriceKobo: in.PriceKobo, PriceUsdCents: in.PriceUSDCents, IsPublished: in.IsPublished,
	})
}

func (s *BookService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteBook(ctx, id)
}

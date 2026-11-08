package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
)

type SettingsService struct {
	queries *sqlc.Queries
}

func NewSettingsService(pool *pgxpool.Pool) *SettingsService {
	return &SettingsService{queries: sqlc.New(pool)}
}

func (s *SettingsService) Get(ctx context.Context) (sqlc.SiteSetting, error) {
	return s.queries.GetSiteSettings(ctx)
}

type UpdateSettingsInput struct {
	PastorName        string
	PastorBio         string
	PastorPhotoURL    string
	CampusesCount     string
	YearsLeadership   string
	ContinentsReached string
	BooksURL          string
	WebsiteURL        string
	ContactEmail      string
	HeroVideoURL      string
}

func (s *SettingsService) Update(ctx context.Context, in UpdateSettingsInput) (sqlc.SiteSetting, error) {
	return s.queries.UpdateSiteSettings(ctx, sqlc.UpdateSiteSettingsParams{
		PastorName:        in.PastorName,
		PastorBio:         in.PastorBio,
		PastorPhotoUrl:    in.PastorPhotoURL,
		CampusesCount:     in.CampusesCount,
		YearsLeadership:   in.YearsLeadership,
		ContinentsReached: in.ContinentsReached,
		BooksUrl:          in.BooksURL,
		WebsiteUrl:        in.WebsiteURL,
		ContactEmail:      in.ContactEmail,
		HeroVideoUrl:      in.HeroVideoURL,
	})
}

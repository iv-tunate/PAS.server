package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
)

type AdminService struct {
	queries *sqlc.Queries
}

func NewAdminService(pool *pgxpool.Pool) *AdminService {
	return &AdminService{queries: sqlc.New(pool)}
}

// DashboardStats are the numbers on the dashboard's front page.
type DashboardStats struct {
	PaidOrders      int64 `json:"paid_orders"`
	PendingOrders   int64 `json:"pending_orders"`
	RevenueKobo     int64 `json:"revenue_kobo"`
	RevenueUSDCents int64 `json:"revenue_usd_cents"`
	Subscribers     int64 `json:"subscribers"`
	UnreadInbox     int64 `json:"unread_inbox"`
	Messages        int64 `json:"messages"`
	Events          int64 `json:"events"`
}

func (s *AdminService) DashboardStats(ctx context.Context) (DashboardStats, error) {
	var st DashboardStats
	var err error

	if st.PaidOrders, err = s.queries.CountOrdersByStatus(ctx, "success"); err != nil {
		return st, err
	}
	if st.PendingOrders, err = s.queries.CountOrdersByStatus(ctx, "pending"); err != nil {
		return st, err
	}
	revenue, err := s.queries.SumRevenueByCurrency(ctx)
	if err != nil {
		return st, err
	}
	for _, r := range revenue {
		switch r.Currency {
		case "NGN":
			st.RevenueKobo = r.Total
		case "USD":
			st.RevenueUSDCents = r.Total
		}
	}
	if st.Subscribers, err = s.queries.CountSubscribers(ctx); err != nil {
		return st, err
	}
	if st.UnreadInbox, err = s.queries.CountUnreadContactMessages(ctx); err != nil {
		return st, err
	}
	if st.Messages, err = s.queries.CountMessages(ctx); err != nil {
		return st, err
	}
	st.Events, err = s.queries.CountEvents(ctx)
	return st, err
}

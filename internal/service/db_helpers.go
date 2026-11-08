package service

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func pgTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

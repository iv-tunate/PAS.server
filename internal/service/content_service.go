package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
)

// ErrUnknownContentKey is returned for any key the website doesn't use.
// Keeping the list closed stops the endpoint becoming a place to stash
// arbitrary data, and a typo in the frontend fails loudly.
var ErrUnknownContentKey = errors.New("unknown content key")

var ErrContentTooLong = errors.New("content is too long for this field")

var ErrContentNotLink = errors.New("this field needs a full https:// link")

const (
	limitShort = 120
	limitLine  = 300
	limitPara  = 4000
	limitURL   = 600
)

// contentLimits lists every editable piece of the site and the longest
// value it may hold. The frontend keeps matching placeholders in
// UI/lib/content-defaults.ts. Add a key in both places to make
// something new editable.
var contentLimits = map[string]int{
	"hero.role":    limitLine,
	"hero.first":   limitShort,
	"hero.last":    limitShort,
	"hero.tagline": limitLine,

	"about.heading":      limitLine,
	"about.roles":        limitLine,
	"about.bio":          limitPara,
	"about.fact.role":    limitLine,
	"about.fact.church":  limitLine,
	"about.fact.also":    limitLine,
	"about.fact.married": limitLine,

	"books.heading": limitLine,
	"books.body":    limitPara,

	"watch.heading": limitLine,
	"watch.body":    limitPara,

	"gateway.heading": limitLine,
	"gateway.body":    limitPara,

	"vision.statement": limitPara,
	"vision.mission":   limitPara,

	"visit.heading":       limitLine,
	"visit.sunday":        limitShort,
	"visit.thursday":      limitShort,
	"visit.address":       limitLine,
	"visit.abuja.times":   limitLine,
	"visit.abuja.address": limitLine,

	"relearn.heading": limitLine,
	"relearn.body":    limitPara,

	"events.heading": limitLine,
	"events.body":    limitPara,

	"contact.heading": limitLine,
	"contact.body":    limitPara,

	"newsletter.heading": limitLine,
	"newsletter.body":    limitPara,

	"messages.heading": limitLine,
	"messages.body":    limitPara,

	"social.instagram": limitURL,
	"social.facebook":  limitURL,
	"social.threads":   limitURL,
	"social.youtube":   limitURL,
	"social.x":         limitURL,

	// Pastor photos (hero, About, full-screen stage) and church photos
	// (the Gateway Nation band).
	"photo.1": limitURL, "photo.2": limitURL, "photo.3": limitURL,
	"photo.4": limitURL, "photo.5": limitURL, "photo.6": limitURL,
	"church.photo.1": limitURL, "church.photo.2": limitURL,
	"church.photo.3": limitURL, "church.photo.4": limitURL,
}

// isLinkKey is true for keys the website renders as a link or an image. Those
// must be https addresses, so nobody can plant a "javascript:" link in the footer.
func isLinkKey(key string) bool {
	return strings.HasPrefix(key, "social.") || strings.HasPrefix(key, "photo.") || strings.HasPrefix(key, "church.photo.")
}

type ContentService struct {
	queries *sqlc.Queries
}

func NewContentService(pool *pgxpool.Pool) *ContentService {
	return &ContentService{queries: sqlc.New(pool)}
}

// All returns every saved value keyed by name. Keys with no row are
// simply absent, which the frontend reads as "use the placeholder".
func (s *ContentService) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.queries.ListSiteContent(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// Set saves a value. An empty (or whitespace-only) value deletes the
// row, which puts the placeholder back.
func (s *ContentService) Set(ctx context.Context, key, value string) error {
	limit, ok := contentLimits[key]
	if !ok {
		return ErrUnknownContentKey
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return s.queries.DeleteSiteContent(ctx, key)
	}
	if utf8.RuneCountInString(value) > limit {
		return ErrContentTooLong
	}
	if isLinkKey(key) {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return ErrContentNotLink
		}
	}

	_, err := s.queries.UpsertSiteContent(ctx, sqlc.UpsertSiteContentParams{Key: key, Value: value})
	return err
}

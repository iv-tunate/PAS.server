package service

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
)

var ErrBadYouTubeLink = errors.New("that does not look like a YouTube link")

var youTubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ParseYouTubeID accepts the ways people copy a YouTube link (watch, share,
// embed, shorts, live) or a bare 11-character id, and returns the id.
func ParseYouTubeID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if youTubeID.MatchString(raw) {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", ErrBadYouTubeLink
	}

	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	var id string
	switch host {
	case "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	case "youtube.com", "m.youtube.com", "music.youtube.com", "youtube-nocookie.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) == 2 && (parts[0] == "embed" || parts[0] == "shorts" || parts[0] == "live" || parts[0] == "v") {
				id = parts[1]
			}
		}
	}
	if !youTubeID.MatchString(id) {
		return "", ErrBadYouTubeLink
	}
	return id, nil
}

type MessageService struct {
	queries *sqlc.Queries
}

func NewMessageService(pool *pgxpool.Pool) *MessageService {
	return &MessageService{queries: sqlc.New(pool)}
}

// Message is a sermon: a title, a YouTube video and (optionally) a date.
type Message struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	YouTubeID   string    `json:"youtube_id"`
	PreachedOn  string    `json:"preached_on"` // YYYY-MM-DD, or empty
	IsPublished bool      `json:"is_published"`
}

func toMessage(m sqlc.Message) Message {
	out := Message{ID: m.ID, Title: m.Title, Description: m.Description, YouTubeID: m.YoutubeID, IsPublished: m.IsPublished}
	if m.PreachedOn.Valid {
		out.PreachedOn = m.PreachedOn.Time.Format("2006-01-02")
	}
	return out
}

func (s *MessageService) ListPublished(ctx context.Context) ([]Message, error) {
	rows, err := s.queries.ListPublishedMessages(ctx)
	if err != nil {
		return nil, err
	}
	return mapMessages(rows), nil
}

func (s *MessageService) ListAll(ctx context.Context) ([]Message, error) {
	rows, err := s.queries.ListAllMessages(ctx)
	if err != nil {
		return nil, err
	}
	return mapMessages(rows), nil
}

func mapMessages(rows []sqlc.Message) []Message {
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMessage(r))
	}
	return out
}

type MessageInput struct {
	Title       string
	Description string
	YouTubeURL  string // a link or an id
	PreachedOn  string // YYYY-MM-DD or empty
	IsPublished bool
}

func (in MessageInput) parse() (youtubeID string, date pgtype.Date, err error) {
	youtubeID, err = ParseYouTubeID(in.YouTubeURL)
	if err != nil {
		return "", pgtype.Date{}, err
	}
	if strings.TrimSpace(in.PreachedOn) != "" {
		t, perr := time.Parse("2006-01-02", strings.TrimSpace(in.PreachedOn))
		if perr != nil {
			return "", pgtype.Date{}, errors.New("date must look like 2026-02-21")
		}
		date = pgtype.Date{Time: t, Valid: true}
	}
	return youtubeID, date, nil
}

func (s *MessageService) Create(ctx context.Context, in MessageInput) (Message, error) {
	id, date, err := in.parse()
	if err != nil {
		return Message{}, err
	}
	m, err := s.queries.CreateMessage(ctx, sqlc.CreateMessageParams{
		Title: in.Title, Description: in.Description, YoutubeID: id, PreachedOn: date, IsPublished: in.IsPublished,
	})
	return toMessage(m), err
}

func (s *MessageService) Update(ctx context.Context, id uuid.UUID, in MessageInput) (Message, error) {
	ytID, date, err := in.parse()
	if err != nil {
		return Message{}, err
	}
	m, err := s.queries.UpdateMessage(ctx, sqlc.UpdateMessageParams{
		ID: id, Title: in.Title, Description: in.Description, YoutubeID: ytID, PreachedOn: date, IsPublished: in.IsPublished,
	})
	return toMessage(m), err
}

func (s *MessageService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteMessage(ctx, id)
}

package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/db/sqlc"
)

type EventService struct {
	queries *sqlc.Queries
}

func NewEventService(pool *pgxpool.Pool) *EventService {
	return &EventService{queries: sqlc.New(pool)}
}

// Event is one yearly programme. Month and days are 0 when not specified;
// the website then shows only the name and the description. There is no year
// on purpose: these come round every year.
type Event struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Month       int16     `json:"month"`
	DayStart    int16     `json:"day_start"`
	DayEnd      int16     `json:"day_end"`
	FlyerURL    string    `json:"flyer_url"`
	Position    int32     `json:"position"`
	IsPublished bool      `json:"is_published"`
}

func toEvent(e sqlc.Event) Event {
	return Event{
		ID: e.ID, Name: e.Name, Description: e.Description, Month: e.Month,
		DayStart: e.DayStart, DayEnd: e.DayEnd, FlyerURL: e.FlyerUrl,
		Position: e.Position, IsPublished: e.IsPublished,
	}
}

func mapEvents(rows []sqlc.Event) []Event {
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEvent(r))
	}
	return out
}

func (s *EventService) ListPublished(ctx context.Context) ([]Event, error) {
	rows, err := s.queries.ListPublishedEvents(ctx)
	if err != nil {
		return nil, err
	}
	return mapEvents(rows), nil
}

func (s *EventService) ListAll(ctx context.Context) ([]Event, error) {
	rows, err := s.queries.ListAllEvents(ctx)
	if err != nil {
		return nil, err
	}
	return mapEvents(rows), nil
}

type EventInput struct {
	Name        string
	Description string
	Month       int16
	DayStart    int16
	DayEnd      int16
	FlyerURL    string
	IsPublished bool
}

// Create puts a new event at the end of the list.
func (s *EventService) Create(ctx context.Context, in EventInput) (Event, error) {
	pos, err := s.queries.NextEventPosition(ctx)
	if err != nil {
		return Event{}, err
	}
	e, err := s.queries.CreateEvent(ctx, sqlc.CreateEventParams{
		Name: in.Name, Description: in.Description, Month: in.Month, DayStart: in.DayStart,
		DayEnd: in.DayEnd, FlyerUrl: in.FlyerURL, Position: pos, IsPublished: in.IsPublished,
	})
	return toEvent(e), err
}

// Update keeps the event where it is in the list; ordering is changed with Move.
func (s *EventService) Update(ctx context.Context, id uuid.UUID, position int32, in EventInput) (Event, error) {
	e, err := s.queries.UpdateEvent(ctx, sqlc.UpdateEventParams{
		ID: id, Name: in.Name, Description: in.Description, Month: in.Month, DayStart: in.DayStart,
		DayEnd: in.DayEnd, FlyerUrl: in.FlyerURL, Position: position, IsPublished: in.IsPublished,
	})
	return toEvent(e), err
}

// Move swaps two events' places. The dashboard calls it for the up/down arrows.
func (s *EventService) Move(ctx context.Context, a, b uuid.UUID, posA, posB int32) error {
	if err := s.queries.SetEventPosition(ctx, sqlc.SetEventPositionParams{ID: a, Position: posB}); err != nil {
		return err
	}
	return s.queries.SetEventPosition(ctx, sqlc.SetEventPositionParams{ID: b, Position: posA})
}

func (s *EventService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteEvent(ctx, id)
}

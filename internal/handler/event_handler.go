package handler

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

type EventHandler struct {
	events *service.EventService
}

func NewEventHandler(events *service.EventService) *EventHandler {
	return &EventHandler{events: events}
}

func (h *EventHandler) List(c echo.Context) error {
	out, err := h.events.ListPublished(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load events")
	}
	return response.OK(c, "", out)
}

func (h *EventHandler) AdminList(c echo.Context) error {
	out, err := h.events.ListAll(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load events")
	}
	return response.OK(c, "", out)
}

type eventRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Month       int16  `json:"month"`
	DayStart    int16  `json:"day_start"`
	DayEnd      int16  `json:"day_end"`
	FlyerURL    string `json:"flyer_url"`
	Position    int32  `json:"position"`
	IsPublished bool   `json:"is_published"`
}

func (r *eventRequest) Validate() error {
	if err := validate.Required(map[string]string{"name": r.Name}); err != nil {
		return err
	}
	if err := validate.MaxLen("name", r.Name, 120); err != nil {
		return err
	}
	if err := validate.MaxLen("description", r.Description, 600); err != nil {
		return err
	}
	if err := validate.HTTPSURL("flyer", r.FlyerURL); err != nil {
		return err
	}
	if r.Month < 0 || r.Month > 12 {
		return errors.New("month must be between 1 and 12")
	}
	if r.DayStart < 0 || r.DayStart > 31 || r.DayEnd < 0 || r.DayEnd > 31 {
		return errors.New("days must be between 1 and 31")
	}
	// No date at all is fine. Days without a month make no sense, and the end
	// of a range cannot come before its start.
	if r.Month == 0 {
		r.DayStart, r.DayEnd = 0, 0
	}
	if r.DayStart == 0 {
		r.DayEnd = 0
	}
	if r.DayEnd != 0 && r.DayEnd < r.DayStart {
		return errors.New("the last day cannot be before the first day")
	}
	return nil
}

func (r *eventRequest) input() service.EventInput {
	return service.EventInput{
		Name: r.Name, Description: r.Description, Month: r.Month, DayStart: r.DayStart,
		DayEnd: r.DayEnd, FlyerURL: r.FlyerURL, IsPublished: r.IsPublished,
	}
}

func (h *EventHandler) AdminCreate(c echo.Context) error {
	var req eventRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	e, err := h.events.Create(c.Request().Context(), req.input())
	if err != nil {
		return response.Internal(c, "could not add the event")
	}
	return response.Created(c, "event added", e)
}

func (h *EventHandler) AdminUpdate(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	var req eventRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	e, err := h.events.Update(c.Request().Context(), id, req.Position, req.input())
	if errors.Is(err, pgx.ErrNoRows) {
		return response.NotFound(c, "event not found")
	}
	if err != nil {
		return response.Internal(c, "could not update the event")
	}
	return response.OK(c, "event updated", e)
}

// AdminMove swaps this event with a neighbour (the up / down arrows).
func (h *EventHandler) AdminMove(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	var req struct {
		OtherID       string `json:"other_id"`
		Position      int32  `json:"position"`
		OtherPosition int32  `json:"other_position"`
	}
	if err := c.Bind(&req); err != nil {
		return response.BadRequest(c, "invalid_request", "could not parse request body", nil)
	}
	other, err := uuid.Parse(req.OtherID)
	if err != nil {
		return response.BadRequest(c, "invalid_id", "invalid id", nil)
	}
	if err := h.events.Move(c.Request().Context(), id, other, req.Position, req.OtherPosition); err != nil {
		return response.Internal(c, "could not reorder events")
	}
	return response.OK(c, "reordered", nil)
}

func (h *EventHandler) AdminDelete(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	if err := h.events.Delete(c.Request().Context(), id); err != nil {
		return response.Internal(c, "could not delete the event")
	}
	return response.NoContent(c)
}

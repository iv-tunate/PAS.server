package handler

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

// MessageHandler serves the sermon library: titles with YouTube links.
type MessageHandler struct {
	messages *service.MessageService
}

func NewMessageHandler(messages *service.MessageService) *MessageHandler {
	return &MessageHandler{messages: messages}
}

func (h *MessageHandler) List(c echo.Context) error {
	out, err := h.messages.ListPublished(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load messages")
	}
	return response.OK(c, "", out)
}

func (h *MessageHandler) AdminList(c echo.Context) error {
	out, err := h.messages.ListAll(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load messages")
	}
	return response.OK(c, "", out)
}

type messageRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	YouTubeURL  string `json:"youtube_url"`
	PreachedOn  string `json:"preached_on"`
	IsPublished bool   `json:"is_published"`
}

func (r *messageRequest) Validate() error {
	if err := validate.Required(map[string]string{"title": r.Title, "youtube_url": r.YouTubeURL}); err != nil {
		return err
	}
	if err := validate.MaxLen("title", r.Title, 200); err != nil {
		return err
	}
	return validate.MaxLen("description", r.Description, 1000)
}

func (r *messageRequest) input() service.MessageInput {
	return service.MessageInput{
		Title: r.Title, Description: r.Description, YouTubeURL: r.YouTubeURL,
		PreachedOn: r.PreachedOn, IsPublished: r.IsPublished,
	}
}

func (h *MessageHandler) AdminCreate(c echo.Context) error {
	var req messageRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	m, err := h.messages.Create(c.Request().Context(), req.input())
	if errors.Is(err, service.ErrBadYouTubeLink) {
		return response.BadRequest(c, "bad_link", err.Error(), nil)
	}
	if err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	return response.Created(c, "message added", m)
}

func (h *MessageHandler) AdminUpdate(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	var req messageRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	m, err := h.messages.Update(c.Request().Context(), id, req.input())
	if errors.Is(err, pgx.ErrNoRows) {
		return response.NotFound(c, "message not found")
	}
	if err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	return response.OK(c, "message updated", m)
}

func (h *MessageHandler) AdminDelete(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	if err := h.messages.Delete(c.Request().Context(), id); err != nil {
		return response.Internal(c, "could not delete message")
	}
	return response.NoContent(c)
}

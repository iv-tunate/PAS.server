package handler

import (
	"errors"

	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
)

type ContentHandler struct {
	content *service.ContentService
}

func NewContentHandler(content *service.ContentService) *ContentHandler {
	return &ContentHandler{content: content}
}

// List is public: every visitor needs the saved copy to render the page.
func (h *ContentHandler) List(c echo.Context) error {
	all, err := h.content.All(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load site content")
	}
	return response.OK(c, "", all)
}

type setContentRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AdminSet saves one field. An empty value resets it to the placeholder.
func (h *ContentHandler) AdminSet(c echo.Context) error {
	var req setContentRequest
	if err := c.Bind(&req); err != nil {
		return response.BadRequest(c, "invalid_request", "could not parse request body", nil)
	}

	err := h.content.Set(c.Request().Context(), req.Key, req.Value)
	switch {
	case errors.Is(err, service.ErrUnknownContentKey):
		return response.BadRequest(c, "unknown_key", "that part of the site can't be edited", nil)
	case errors.Is(err, service.ErrContentNotLink):
		return response.BadRequest(c, "not_a_link", "this field needs a full https:// link", nil)
	case errors.Is(err, service.ErrContentTooLong):
		return response.BadRequest(c, "too_long", "that text is too long for this field", nil)
	case err != nil:
		return response.Internal(c, "could not save content")
	}
	return response.OK(c, "saved", map[string]string{"key": req.Key})
}

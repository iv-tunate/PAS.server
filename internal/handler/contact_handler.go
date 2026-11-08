package handler

import (
	"errors"

	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

// ContactHandler takes messages and appointment requests from the website.
type ContactHandler struct {
	contact *service.ContactService
}

func NewContactHandler(contact *service.ContactService) *ContactHandler {
	return &ContactHandler{contact: contact}
}

type contactRequest struct {
	Topic         string `json:"topic"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	PreferredTime string `json:"preferred_time"`
	Body          string `json:"body"`
	// Website is a trap. It is hidden from people, so only bots fill it in.
	Website string `json:"website"`
}

func (r *contactRequest) Validate() error {
	if r.Topic == "" {
		r.Topic = "message"
	}
	if !service.ValidTopic(r.Topic) {
		return errors.New("choose what this is about")
	}
	if len([]rune(r.Name)) < 2 {
		return errors.New("please enter your name")
	}
	if err := validate.MaxLen("name", r.Name, 100); err != nil {
		return err
	}
	if err := validate.Email(r.Email); err != nil {
		return err
	}
	if r.Phone != "" {
		if err := validate.Phone(r.Phone); err != nil {
			return err
		}
	}
	if err := validate.MaxLen("preferred time", r.PreferredTime, 100); err != nil {
		return err
	}
	if len([]rune(r.Body)) < 5 {
		return errors.New("please write a short message")
	}
	return validate.MaxLen("message", r.Body, 4000)
}

func (h *ContactHandler) Create(c echo.Context) error {
	var req contactRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}

	// A bot that fills the hidden field is told it worked and nothing is stored.
	if req.Website != "" {
		return response.OK(c, "message sent", nil)
	}

	err := h.contact.Create(c.Request().Context(), service.ContactInput{
		Topic: req.Topic, Name: req.Name, Email: req.Email, Phone: req.Phone,
		PreferredTime: req.PreferredTime, Body: req.Body,
	})
	if err != nil {
		return response.Internal(c, "could not send your message, please try again")
	}
	return response.OK(c, "message sent", nil)
}

// ---------------------------------------------------------------------
// Admin inbox
// ---------------------------------------------------------------------

func (h *ContactHandler) AdminList(c echo.Context) error {
	limit, offset := pageParams(c)
	rows, err := h.contact.List(c.Request().Context(), limit, offset)
	if err != nil {
		return response.Internal(c, "could not load the inbox")
	}
	return response.OK(c, "", rows)
}

func (h *ContactHandler) AdminSetRead(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	var req struct {
		IsRead bool `json:"is_read"`
	}
	if err := c.Bind(&req); err != nil {
		return response.BadRequest(c, "invalid_request", "could not parse request body", nil)
	}
	if err := h.contact.SetRead(c.Request().Context(), id, req.IsRead); err != nil {
		return response.Internal(c, "could not update the message")
	}
	return response.OK(c, "updated", nil)
}

func (h *ContactHandler) AdminDelete(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	if err := h.contact.Delete(c.Request().Context(), id); err != nil {
		return response.Internal(c, "could not delete the message")
	}
	return response.NoContent(c)
}

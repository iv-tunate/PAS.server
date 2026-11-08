package handler

import (
	"errors"

	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/middleware"
	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

type NewsletterHandler struct {
	newsletter *service.NewsletterService
	auth       *service.AuthService
}

func NewNewsletterHandler(newsletter *service.NewsletterService, auth *service.AuthService) *NewsletterHandler {
	return &NewsletterHandler{newsletter: newsletter, auth: auth}
}

type subscribeRequest struct {
	Email string `json:"email"`
}

func (r *subscribeRequest) Validate() error { return validate.Email(r.Email) }

func (h *NewsletterHandler) Subscribe(c echo.Context) error {
	var req subscribeRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	if err := h.newsletter.Subscribe(c.Request().Context(), req.Email); err != nil {
		return response.Internal(c, "could not subscribe right now")
	}
	return response.OK(c, "subscribed", nil)
}

type unsubscribeRequest struct {
	Token string `json:"token"`
}

func (r *unsubscribeRequest) Validate() error {
	if err := validate.Required(map[string]string{"token": r.Token}); err != nil {
		return err
	}
	return validate.MaxLen("token", r.Token, 64)
}

// Unsubscribe is a POST (not a link that does it on a GET) so mail scanners
// that "click" every link in an email cannot unsubscribe people by accident.
func (h *NewsletterHandler) Unsubscribe(c echo.Context) error {
	var req unsubscribeRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	if err := h.newsletter.Unsubscribe(c.Request().Context(), req.Token); err != nil {
		return response.Internal(c, "could not unsubscribe right now")
	}
	return response.OK(c, "unsubscribed", nil)
}

// ---------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------

type subscriberView struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

func (h *NewsletterHandler) AdminSubscribers(c echo.Context) error {
	limit, offset := pageParams(c)
	rows, total, err := h.newsletter.ListSubscribers(c.Request().Context(), limit, offset)
	if err != nil {
		return response.Internal(c, "could not load subscribers")
	}
	out := make([]subscriberView, 0, len(rows))
	for _, r := range rows {
		out = append(out, subscriberView{ID: r.ID.String(), Email: r.Email, CreatedAt: r.CreatedAt.Time.Format("2006-01-02")})
	}
	return response.OKWithMeta(c, "", out, &response.Meta{
		Page: int(offset/limit) + 1, PerPage: int(limit), TotalCount: int(total),
	})
}

func (h *NewsletterHandler) AdminDeleteSubscriber(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	if err := h.newsletter.DeleteSubscriber(c.Request().Context(), id); err != nil {
		return response.Internal(c, "could not remove the subscriber")
	}
	return response.NoContent(c)
}

func (h *NewsletterHandler) AdminCampaigns(c echo.Context) error {
	rows, err := h.newsletter.ListCampaigns(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load past newsletters")
	}
	return response.OK(c, "", rows)
}

type campaignRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (r *campaignRequest) Validate() error {
	if err := validate.Required(map[string]string{"subject": r.Subject, "body": r.Body}); err != nil {
		return err
	}
	if err := validate.MaxLen("subject", r.Subject, 150); err != nil {
		return err
	}
	return validate.MaxLen("message", r.Body, 20000)
}

// AdminSend starts a broadcast to every subscriber and returns at once; the
// sending carries on in the background.
func (h *NewsletterHandler) AdminSend(c echo.Context) error {
	var req campaignRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	campaign, err := h.newsletter.StartCampaign(c.Request().Context(), req.Subject, req.Body)
	switch {
	case errors.Is(err, service.ErrNoSubscribers):
		return response.BadRequest(c, "no_subscribers", err.Error(), nil)
	case err != nil:
		return response.Internal(c, "could not start sending")
	}
	return response.Created(c, "sending started", campaign)
}

// AdminSendTest mails the draft to the admin who is logged in.
func (h *NewsletterHandler) AdminSendTest(c echo.Context) error {
	var req campaignRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	user, err := h.auth.GetByID(c.Request().Context(), middleware.Claims(c).UserID)
	if err != nil {
		return response.Unauthorized(c, "account not found")
	}
	if err := h.newsletter.SendTest(c.Request().Context(), user.Email, req.Subject, req.Body); err != nil {
		return response.Fail(c, 503, "busy", err.Error(), nil)
	}
	return response.OK(c, "test sent to "+user.Email, nil)
}

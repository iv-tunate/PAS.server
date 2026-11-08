package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

// OrderHandler is the public side of buying (or getting free) a book or
// course: no login, just name, email and phone.
type OrderHandler struct {
	orders *service.OrderService
}

func NewOrderHandler(orders *service.OrderService) *OrderHandler {
	return &OrderHandler{orders: orders}
}

// Gateways tells the website which payment methods are switched on.
func (h *OrderHandler) Gateways(c echo.Context) error {
	return response.OK(c, "", echo.Map{"gateways": h.orders.Gateways()})
}

type checkoutRequest struct {
	Kind      string `json:"kind"`
	ItemID    string `json:"item_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Gateway   string `json:"gateway"`
	Subscribe bool   `json:"subscribe"`

	itemID uuid.UUID
}

func (r *checkoutRequest) Validate() error {
	if r.Kind != "book" && r.Kind != "course" {
		return errors.New("kind must be book or course")
	}
	id, err := uuid.Parse(r.ItemID)
	if err != nil {
		return errors.New("item_id is not valid")
	}
	r.itemID = id

	if len([]rune(r.Name)) < 2 {
		return errors.New("please enter your name")
	}
	if err := validate.MaxLen("name", r.Name, 100); err != nil {
		return err
	}
	if err := validate.Email(r.Email); err != nil {
		return err
	}
	if err := validate.Phone(r.Phone); err != nil {
		return err
	}
	if r.Gateway != "" && r.Gateway != "paystack" && r.Gateway != "flutterwave" && r.Gateway != "stripe" {
		return errors.New("unknown payment method")
	}
	return nil
}

func (h *OrderHandler) Checkout(c echo.Context) error {
	var req checkoutRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}

	res, err := h.orders.Checkout(c.Request().Context(), service.CheckoutInput{
		Kind: req.Kind, ItemID: req.itemID, Name: req.Name, Email: req.Email,
		Phone: req.Phone, Gateway: req.Gateway, Subscribe: req.Subscribe,
	})
	switch {
	case errors.Is(err, service.ErrItemNotFound):
		return response.NotFound(c, "that item could not be found")
	case errors.Is(err, service.ErrNotAvailable):
		return response.Conflict(c, "not_available", "this is not available yet, please check back soon")
	case errors.Is(err, service.ErrNotForSale):
		return response.BadRequest(c, "no_price", "this item can't be bought with that payment method", nil)
	case errors.Is(err, service.ErrNoGateway):
		return response.Fail(c, http.StatusServiceUnavailable, "payments_unavailable", "payments are not available right now, please try again later", nil)
	case errors.Is(err, service.ErrGatewayFailed):
		return response.Fail(c, http.StatusBadGateway, "gateway_error", "we could not start the payment, please try again", nil)
	case errors.Is(err, service.ErrTooManyOrders):
		return response.Fail(c, http.StatusTooManyRequests, "too_many", err.Error(), nil)
	case err != nil:
		return response.Internal(c, "could not complete your request")
	}

	if res.Free {
		return response.OK(c, "check your email", res)
	}
	return response.OK(c, "continue to payment", res)
}

// Status backs the "thank you" page that buyers land on after paying.
func (h *OrderHandler) Status(c echo.Context) error {
	st, err := h.orders.Status(c.Request().Context(), c.Param("reference"))
	if err != nil {
		return response.NotFound(c, "order not found")
	}
	return response.OK(c, "", st)
}

// Download is the link in the delivery email. It redirects to the real file.
func (h *OrderHandler) Download(c echo.Context) error {
	url, err := h.orders.Download(c.Request().Context(), c.Param("token"))
	if err != nil {
		return c.HTML(http.StatusNotFound,
			`<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><body style="font-family:sans-serif;max-width:32rem;margin:15vh auto;padding:0 1rem"><h1>This link is not valid</h1><p>It may have been copied wrongly, or the file is no longer available. Please reply to your email or contact us and we will send it again.</p></body>`)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Referrer-Policy", "no-referrer")
	return c.Redirect(http.StatusFound, url)
}

// Webhook receives payment notifications from Paystack, Flutterwave and
// Stripe at /payments/webhook/:gateway.
func (h *OrderHandler) Webhook(c echo.Context) error {
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20))
	if err != nil {
		return response.BadRequest(c, "invalid_body", "could not read request body", nil)
	}
	err = h.orders.HandleWebhook(c.Request().Context(), c.Param("gateway"), c.Request().Header, body)
	switch {
	case errors.Is(err, service.ErrNoGateway):
		return response.NotFound(c, "unknown gateway")
	case err != nil:
		return response.Unauthorized(c, "invalid webhook")
	}
	return response.OK(c, "received", nil)
}

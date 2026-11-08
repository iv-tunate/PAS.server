package handler

import (
	"errors"

	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
)

// AdminHandler serves the dashboard's overview and orders.
type AdminHandler struct {
	admin  *service.AdminService
	orders *service.OrderService
}

func NewAdminHandler(admin *service.AdminService, orders *service.OrderService) *AdminHandler {
	return &AdminHandler{admin: admin, orders: orders}
}

func (h *AdminHandler) DashboardStats(c echo.Context) error {
	stats, err := h.admin.DashboardStats(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load dashboard stats")
	}
	return response.OK(c, "", stats)
}

func (h *AdminHandler) ListOrders(c echo.Context) error {
	status := c.QueryParam("status")
	if status != "" && status != "pending" && status != "success" && status != "failed" {
		return response.BadRequest(c, "invalid_status", "status must be pending, success or failed", nil)
	}
	limit, offset := pageParams(c)
	orders, total, err := h.orders.ListForAdmin(c.Request().Context(), status, limit, offset)
	if err != nil {
		return response.Internal(c, "could not load orders")
	}
	return response.OKWithMeta(c, "", orders, &response.Meta{
		Page: int(offset/limit) + 1, PerPage: int(limit), TotalCount: int(total),
	})
}

func (h *AdminHandler) ResendOrder(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	err := h.orders.Resend(c.Request().Context(), id)
	switch {
	case errors.Is(err, service.ErrOrderNotFound):
		return response.NotFound(c, "order not found")
	case errors.Is(err, service.ErrNotPaid):
		return response.BadRequest(c, "not_paid", "this order has not been paid yet", nil)
	case err != nil:
		return response.Internal(c, "could not resend the email")
	}
	return response.OK(c, "email queued", nil)
}

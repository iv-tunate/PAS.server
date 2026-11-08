package handler

import (
	"github.com/labstack/echo/v4"

	custommw "github.com/iv-tunate/PAS/internal/middleware"
	"github.com/iv-tunate/PAS/internal/service"
)

type Handlers struct {
	Auth       *AuthHandler
	Course     *CourseHandler
	Book       *BookHandler
	Order      *OrderHandler
	Message    *MessageHandler
	Event      *EventHandler
	Contact    *ContactHandler
	Newsletter *NewsletterHandler
	Admin      *AdminHandler
	Settings   *SettingsHandler
	Content    *ContentHandler
	Upload     *UploadHandler
}

// Limiters are the per-address rate limits for the routes that anyone on the
// internet can hit and that cost something (an email, a database write, a
// call to a payment provider). server.go builds them.
type Limiters struct {
	Login  echo.MiddlewareFunc // password guessing
	Form   echo.MiddlewareFunc // checkout, contact, newsletter
	Lookup echo.MiddlewareFunc // order status, downloads
}

func RegisterRoutes(e *echo.Echo, h Handlers, lim Limiters, authService *service.AuthService, frontendURL string) {
	api := e.Group("/api/v1")

	// --- Admin sign-in (there are no visitor accounts) ---
	auth := api.Group("/auth")
	auth.POST("/login", h.Auth.Login, lim.Login)
	auth.POST("/refresh", h.Auth.Refresh, lim.Login)
	auth.POST("/logout", h.Auth.Logout)
	auth.POST("/forgot-password", func(c echo.Context) error {
		return h.Auth.ForgotPassword(c, frontendURL+"/admin/reset-password")
	}, lim.Login)
	auth.POST("/reset-password", h.Auth.ResetPassword, lim.Login)

	// --- Public: read ---
	api.GET("/content", h.Content.List)
	api.GET("/settings", h.Settings.Get)
	api.GET("/books", h.Book.List)
	api.GET("/courses", h.Course.List)
	api.GET("/courses/:slug", h.Course.GetBySlug)
	api.GET("/messages", h.Message.List)
	api.GET("/events", h.Event.List)
	api.GET("/checkout/gateways", h.Order.Gateways)

	// --- Public: act ---
	api.POST("/checkout", h.Order.Checkout, lim.Form)
	api.GET("/orders/:reference/status", h.Order.Status, lim.Lookup)
	api.GET("/download/:token", h.Order.Download, lim.Lookup)
	api.POST("/contact", h.Contact.Create, lim.Form)
	api.POST("/newsletter/subscribe", h.Newsletter.Subscribe, lim.Form)
	api.POST("/newsletter/unsubscribe", h.Newsletter.Unsubscribe, lim.Form)

	// Payment providers call these. They are protected by the provider's
	// signature, not a login.
	api.POST("/payments/webhook/:gateway", h.Order.Webhook)

	// --- Signed in ---
	requireAuth := custommw.RequireAuth(authService)
	api.GET("/me", h.Auth.Me, requireAuth)

	// --- Admin only ---
	admin := api.Group("/admin", requireAuth, custommw.RequireAdmin)
	admin.GET("/stats", h.Admin.DashboardStats)

	admin.GET("/orders", h.Admin.ListOrders)
	admin.POST("/orders/:id/resend", h.Admin.ResendOrder)

	admin.GET("/books", h.Book.AdminList)
	admin.POST("/books", h.Book.AdminCreate)
	admin.PUT("/books/:id", h.Book.AdminUpdate)
	admin.DELETE("/books/:id", h.Book.AdminDelete)

	admin.GET("/courses", h.Course.AdminList)
	admin.POST("/courses", h.Course.AdminCreate)
	admin.PUT("/courses/:id", h.Course.AdminUpdate)
	admin.DELETE("/courses/:id", h.Course.AdminDelete)

	admin.GET("/messages", h.Message.AdminList)
	admin.POST("/messages", h.Message.AdminCreate)
	admin.PUT("/messages/:id", h.Message.AdminUpdate)
	admin.DELETE("/messages/:id", h.Message.AdminDelete)

	admin.GET("/events", h.Event.AdminList)
	admin.POST("/events", h.Event.AdminCreate)
	admin.PUT("/events/:id", h.Event.AdminUpdate)
	admin.POST("/events/:id/move", h.Event.AdminMove)
	admin.DELETE("/events/:id", h.Event.AdminDelete)

	admin.GET("/inbox", h.Contact.AdminList)
	admin.PUT("/inbox/:id/read", h.Contact.AdminSetRead)
	admin.DELETE("/inbox/:id", h.Contact.AdminDelete)

	admin.GET("/subscribers", h.Newsletter.AdminSubscribers)
	admin.DELETE("/subscribers/:id", h.Newsletter.AdminDeleteSubscriber)
	admin.GET("/newsletters", h.Newsletter.AdminCampaigns)
	admin.POST("/newsletters", h.Newsletter.AdminSend)
	admin.POST("/newsletters/test", h.Newsletter.AdminSendTest)

	admin.GET("/admins", h.Auth.ListAdmins)
	admin.POST("/admins", h.Auth.CreateAdmin)
	admin.DELETE("/admins/:id", h.Auth.DeleteAdmin)
	admin.PUT("/me/password", h.Auth.ChangePassword)

	admin.PUT("/settings", h.Settings.AdminUpdate)
	admin.PUT("/content", h.Content.AdminSet)
	admin.GET("/uploads/signature", h.Upload.Sign)
}

package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	"github.com/iv-tunate/PAS/internal/config"
	"github.com/iv-tunate/PAS/internal/handler"
	"github.com/iv-tunate/PAS/internal/mailer"
	custommw "github.com/iv-tunate/PAS/internal/middleware"
	"github.com/iv-tunate/PAS/internal/payment"
	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/internal/upload"
)

type App struct {
	Echo       *echo.Echo
	outbox     *mailer.Outbox
	orders     *service.OrderService
	newsletter *service.NewsletterService
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool) *App {
	brevo := mailer.NewBrevoMailer(cfg.BrevoAPIKey, cfg.BrevoSenderEmail, cfg.BrevoSenderName)
	outbox := mailer.NewOutbox(brevo, log, 4, 500)
	signer := upload.NewCloudinarySigner(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret)

	var gateways []payment.Gateway
	if cfg.PaystackSecretKey != "" {
		gateways = append(gateways, payment.NewPaystack(cfg.PaystackSecretKey))
	}
	if cfg.FlutterwaveSecretKey != "" {
		gateways = append(gateways, payment.NewFlutterwave(cfg.FlutterwaveSecretKey, cfg.FlutterwaveWebhookSecret))
	}
	if cfg.StripeSecretKey != "" {
		gateways = append(gateways, payment.NewStripe(cfg.StripeSecretKey, cfg.StripeWebhookSecret))
	}
	if len(gateways) == 0 {
		log.Warn("no payment gateway is configured; paid items cannot be bought (free ones still work)")
	}

	authService := service.NewAuthService(pool, cfg, log, outbox)
	orderService := service.NewOrderService(pool, cfg, log, outbox, gateways...)
	newsletterService := service.NewNewsletterService(ctx, pool, brevo, outbox, cfg, log)

	handlers := handler.Handlers{
		Auth:       handler.NewAuthHandler(authService),
		Course:     handler.NewCourseHandler(service.NewCourseService(pool)),
		Book:       handler.NewBookHandler(service.NewBookService(pool)),
		Order:      handler.NewOrderHandler(orderService),
		Message:    handler.NewMessageHandler(service.NewMessageService(pool)),
		Event:      handler.NewEventHandler(service.NewEventService(pool)),
		Contact:    handler.NewContactHandler(service.NewContactService(pool, outbox, log)),
		Newsletter: handler.NewNewsletterHandler(newsletterService, authService),
		Admin:      handler.NewAdminHandler(service.NewAdminService(pool), orderService),
		Settings:   handler.NewSettingsHandler(service.NewSettingsService(pool)),
		Content:    handler.NewContentHandler(service.NewContentService(pool)),
		Upload:     handler.NewUploadHandler(signer),
	}

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = errorHandler(log)

	if cfg.TrustProxy {
		e.IPExtractor = echo.ExtractIPFromXFFHeader()
	} else {
		e.IPExtractor = echo.ExtractIPDirect()
	}

	e.Use(echomw.RequestID())
	e.Use(custommw.RequestLogger(log))
	e.Use(echomw.Recover())
	e.Use(echomw.SecureWithConfig(echomw.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
	}))

	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: []string{cfg.FrontendURL},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAuthorization},
	}))

	e.Use(limit(20*60, 40, 3*time.Minute))
	e.Use(echomw.BodyLimit("2M"))

	handler.RegisterRoutes(e, handlers, handler.Limiters{
		Login:  limit(12, 8, 15*time.Minute),
		Form:   limit(10, 5, 15*time.Minute),
		Lookup: limit(60, 20, 10*time.Minute),
	}, authService, cfg.FrontendURL)

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
	})

	return &App{Echo: e, outbox: outbox, orders: orderService, newsletter: newsletterService}
}

// Start launches the background jobs. Call it once, after New.
func (a *App) Start(ctx context.Context) {
	a.newsletter.MarkInterrupted(ctx)
	go a.orders.RunSweeper(ctx)
}

// Shutdown stops accepting requests, then gives queued emails a moment to go out.
func (a *App) Shutdown(ctx context.Context, pool *pgxpool.Pool) error {
	err := a.Echo.Shutdown(ctx)
	a.outbox.Close(ctx)
	pool.Close()
	return err
}

// limit builds a per-address rate limiter: perMinute requests a minute on
// average, with a burst allowance.
func limit(perMinute float64, burst int, forget time.Duration) echo.MiddlewareFunc {
	return echomw.RateLimiterWithConfig(echomw.RateLimiterConfig{
		Store: echomw.NewRateLimiterMemoryStoreWithConfig(echomw.RateLimiterMemoryStoreConfig{
			Rate:      rate.Limit(perMinute / 60),
			Burst:     burst,
			ExpiresIn: forget,
		}),
		IdentifierExtractor: func(c echo.Context) (string, error) { return c.RealIP(), nil },
		DenyHandler: func(c echo.Context, _ string, _ error) error {
			return c.JSON(http.StatusTooManyRequests, echo.Map{
				"success": false,
				"error":   echo.Map{"code": "rate_limited", "message": "too many attempts, please wait a moment and try again"},
			})
		},
	})
}

func errorHandler(log *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		code := http.StatusInternalServerError
		message := "internal server error"

		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			if msg, ok := he.Message.(string); ok {
				message = msg
			}
		} else {
			log.Error("unhandled error", "error", err, "path", c.Path())
		}

		_ = c.JSON(code, echo.Map{
			"success": false,
			"error":   echo.Map{"code": "error", "message": message},
		})
	}
}

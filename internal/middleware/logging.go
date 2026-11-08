package middleware

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
)

func RequestLogger(log *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)

			status := c.Response().Status
			fields := []any{
				"method", c.Request().Method,
				"path", c.Path(),
				"status", status,
				"latency_ms", time.Since(start).Milliseconds(),
				"ip", c.RealIP(),
				"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
			}

			if err != nil {
				fields = append(fields, "error", err.Error())
			}

			if status >= 500 {
				log.Error("request", fields...)
			} else {
				log.Info("request", fields...)
			}

			return err
		}
	}
}

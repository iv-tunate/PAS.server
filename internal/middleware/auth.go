package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
)

const ContextKeyClaims = "auth_claims"

func RequireAuth(authService *service.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				return response.Unauthorized(c, "missing or malformed authorization header")
			}

			raw := strings.TrimPrefix(header, "Bearer ")
			claims, err := authService.ParseAccessToken(raw)
			if err != nil {
				return response.Unauthorized(c, "invalid or expired session — please log in again")
			}

			c.Set(ContextKeyClaims, claims)
			return next(c)
		}
	}
}

func RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		claims, ok := c.Get(ContextKeyClaims).(*service.Claims)
		if !ok {
			return response.Unauthorized(c, "authentication required")
		}
		if claims.Role != "admin" {
			return response.Forbidden(c, "admin access required")
		}
		return next(c)
	}
}

func Claims(c echo.Context) *service.Claims {
	claims, _ := c.Get(ContextKeyClaims).(*service.Claims)
	return claims
}

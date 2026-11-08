package response

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type Envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   *Error `json:"error,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type Meta struct {
	Page       int `json:"page,omitempty"`
	PerPage    int `json:"per_page,omitempty"`
	TotalCount int `json:"total_count,omitempty"`
}

func OK(c echo.Context, message string, data any) error {
	return c.JSON(http.StatusOK, Envelope{Success: true, Message: message, Data: data})
}

func OKWithMeta(c echo.Context, message string, data any, meta *Meta) error {
	return c.JSON(http.StatusOK, Envelope{Success: true, Message: message, Data: data, Meta: meta})
}

func Created(c echo.Context, message string, data any) error {
	return c.JSON(http.StatusCreated, Envelope{Success: true, Message: message, Data: data})
}

func NoContent(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func Fail(c echo.Context, status int, code, message string, details any) error {
	return c.JSON(status, Envelope{
		Success: false,
		Error:   &Error{Code: code, Message: message, Details: details},
	})
}

func BadRequest(c echo.Context, code, message string, details any) error {
	return Fail(c, http.StatusBadRequest, code, message, details)
}

func Unauthorized(c echo.Context, message string) error {
	return Fail(c, http.StatusUnauthorized, "unauthorized", message, nil)
}

func Forbidden(c echo.Context, message string) error {
	return Fail(c, http.StatusForbidden, "forbidden", message, nil)
}

func NotFound(c echo.Context, message string) error {
	return Fail(c, http.StatusNotFound, "not_found", message, nil)
}

func Conflict(c echo.Context, code, message string) error {
	return Fail(c, http.StatusConflict, code, message, nil)
}

func Internal(c echo.Context, message string) error {
	return Fail(c, http.StatusInternalServerError, "internal_error", message, nil)
}

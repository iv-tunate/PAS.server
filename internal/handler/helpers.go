package handler

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/pkg/response"
)

// idParam reads the :id in the path. When it is not a valid id it writes the
// 400 itself and reports false, so the caller can simply `return nil`.
func idParam(c echo.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = response.BadRequest(c, "invalid_id", "invalid id", nil)
		return uuid.Nil, false
	}
	return id, true
}

func pageParams(c echo.Context) (limit, offset int32) {
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page <= 0 {
		page = 1
	}
	return int32(perPage), int32((page - 1) * perPage)
}

package handler

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

type CourseHandler struct {
	courses *service.CourseService
}

func NewCourseHandler(courses *service.CourseService) *CourseHandler {
	return &CourseHandler{courses: courses}
}

func (h *CourseHandler) List(c echo.Context) error {
	courses, err := h.courses.ListPublished(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load courses")
	}
	return response.OK(c, "", courses)
}

func (h *CourseHandler) GetBySlug(c echo.Context) error {
	course, err := h.courses.GetBySlug(c.Request().Context(), c.Param("slug"))
	if err != nil {
		return response.NotFound(c, "course not found")
	}
	return response.OK(c, "", course)
}

func (h *CourseHandler) AdminList(c echo.Context) error {
	courses, err := h.courses.ListAllForAdmin(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load courses")
	}
	return response.OK(c, "", courses)
}

type courseRequest struct {
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	ThumbnailURL  string   `json:"thumbnail_url"`
	MediaKind     string   `json:"media_kind"`
	FileURL       string   `json:"file_url"`
	IsFree        bool     `json:"is_free"`
	PriceKobo     int64    `json:"price_kobo"`
	PriceUSDCents int64    `json:"price_usd_cents"`
	IsPublished   bool     `json:"is_published"`
	Chapters      []string `json:"chapters"`
}

func (r *courseRequest) Validate() error {
	if err := validate.Required(map[string]string{"title": r.Title}); err != nil {
		return err
	}
	if err := validate.MaxLen("title", r.Title, 200); err != nil {
		return err
	}
	if err := validate.MaxLen("summary", r.Summary, 4000); err != nil {
		return err
	}
	if r.MediaKind == "" {
		r.MediaKind = "video"
	}
	if r.MediaKind != "video" && r.MediaKind != "audio" && r.MediaKind != "file" {
		return errors.New("media kind must be video, audio or file")
	}
	if err := validate.HTTPSURL("thumbnail", r.ThumbnailURL); err != nil {
		return err
	}
	if err := validate.HTTPSURL("course file", r.FileURL); err != nil {
		return err
	}
	if len(r.Chapters) > 60 {
		return errors.New("a course can have at most 60 chapters")
	}
	for _, ch := range r.Chapters {
		if err := validate.MaxLen("chapter title", ch, 200); err != nil {
			return err
		}
	}
	return checkPrice(r.IsFree, &r.PriceKobo, &r.PriceUSDCents)
}

func (r *courseRequest) input() service.CourseInput {
	return service.CourseInput{
		Title: r.Title, Summary: r.Summary, ThumbnailURL: r.ThumbnailURL, MediaKind: r.MediaKind,
		FileURL: r.FileURL, IsFree: r.IsFree, PriceKobo: r.PriceKobo, PriceUSDCents: r.PriceUSDCents,
		IsPublished: r.IsPublished, Chapters: r.Chapters,
	}
}

func (h *CourseHandler) AdminCreate(c echo.Context) error {
	var req courseRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	course, err := h.courses.Create(c.Request().Context(), req.input())
	if err != nil {
		return response.Internal(c, "could not create course")
	}
	return response.Created(c, "course created", course)
}

func (h *CourseHandler) AdminUpdate(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	var req courseRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	course, err := h.courses.Update(c.Request().Context(), id, req.input())
	if errors.Is(err, pgx.ErrNoRows) {
		return response.NotFound(c, "course not found")
	}
	if err != nil {
		return response.Internal(c, "could not update course")
	}
	return response.OK(c, "course updated", course)
}

func (h *CourseHandler) AdminDelete(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	if err := h.courses.Delete(c.Request().Context(), id); err != nil {
		return response.Internal(c, "could not delete course")
	}
	return response.NoContent(c)
}

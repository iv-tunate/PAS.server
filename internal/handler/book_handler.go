package handler

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

type BookHandler struct {
	books *service.BookService
}

func NewBookHandler(books *service.BookService) *BookHandler {
	return &BookHandler{books: books}
}

func (h *BookHandler) List(c echo.Context) error {
	books, err := h.books.ListPublished(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load books")
	}
	return response.OK(c, "", books)
}

func (h *BookHandler) AdminList(c echo.Context) error {
	books, err := h.books.ListAllForAdmin(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load books")
	}
	return response.OK(c, "", books)
}

type bookRequest struct {
	Title         string `json:"title"`
	Author        string `json:"author"`
	Description   string `json:"description"`
	CoverImageURL string `json:"cover_image_url"`
	FileURL       string `json:"file_url"`
	IsFree        bool   `json:"is_free"`
	PriceKobo     int64  `json:"price_kobo"`
	PriceUSDCents int64  `json:"price_usd_cents"`
	IsPublished   bool   `json:"is_published"`
}

func (r *bookRequest) Validate() error {
	if err := validate.Required(map[string]string{"title": r.Title}); err != nil {
		return err
	}
	if err := validate.MaxLen("title", r.Title, 200); err != nil {
		return err
	}
	if err := validate.MaxLen("author", r.Author, 120); err != nil {
		return err
	}
	if err := validate.MaxLen("description", r.Description, 4000); err != nil {
		return err
	}
	if err := validate.HTTPSURL("cover image", r.CoverImageURL); err != nil {
		return err
	}
	if err := validate.HTTPSURL("book file", r.FileURL); err != nil {
		return err
	}
	return checkPrice(r.IsFree, &r.PriceKobo, &r.PriceUSDCents)
}

// checkPrice enforces the free/paid choice. A free item never carries a
// price; a paid one needs at least one (naira for Paystack and Flutterwave,
// dollars for Stripe).
func checkPrice(isFree bool, kobo, usdCents *int64) error {
	if isFree {
		*kobo, *usdCents = 0, 0
		return nil
	}
	if *kobo < 0 || *usdCents < 0 || *kobo > 100_000_000_00 || *usdCents > 10_000_000 {
		return errors.New("that price is not valid")
	}
	if *kobo == 0 && *usdCents == 0 {
		return errors.New("set a price, or mark it as free")
	}
	return nil
}

func (r *bookRequest) input() service.BookInput {
	author := r.Author
	if author == "" {
		author = "Akintola Samuel"
	}
	return service.BookInput{
		Title: r.Title, Author: author, Description: r.Description,
		CoverImageURL: r.CoverImageURL, FileURL: r.FileURL, IsFree: r.IsFree,
		PriceKobo: r.PriceKobo, PriceUSDCents: r.PriceUSDCents, IsPublished: r.IsPublished,
	}
}

func (h *BookHandler) AdminCreate(c echo.Context) error {
	var req bookRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	book, err := h.books.Create(c.Request().Context(), req.input())
	if err != nil {
		return response.Internal(c, "could not create book")
	}
	return response.Created(c, "book created", book)
}

func (h *BookHandler) AdminUpdate(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	var req bookRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	book, err := h.books.Update(c.Request().Context(), id, req.input())
	if errors.Is(err, pgx.ErrNoRows) {
		return response.NotFound(c, "book not found")
	}
	if err != nil {
		return response.Internal(c, "could not update book")
	}
	return response.OK(c, "book updated", book)
}

func (h *BookHandler) AdminDelete(c echo.Context) error {
	id, ok := idParam(c)
	if !ok {
		return nil
	}
	if err := h.books.Delete(c.Request().Context(), id); err != nil {
		return response.Internal(c, "could not delete book")
	}
	return response.NoContent(c)
}

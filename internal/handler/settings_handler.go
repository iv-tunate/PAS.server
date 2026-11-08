package handler

import (
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

type SettingsHandler struct {
	settings *service.SettingsService
}

func NewSettingsHandler(settings *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{settings: settings}
}

func (h *SettingsHandler) Get(c echo.Context) error {
	settings, err := h.settings.Get(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load site settings")
	}
	return response.OK(c, "", settings)
}

type updateSettingsRequest struct {
	PastorName        string `json:"pastor_name"`
	PastorBio         string `json:"pastor_bio"`
	PastorPhotoURL    string `json:"pastor_photo_url"`
	CampusesCount     string `json:"campuses_count"`
	YearsLeadership   string `json:"years_leadership"`
	ContinentsReached string `json:"continents_reached"`
	BooksURL          string `json:"books_url"`
	WebsiteURL        string `json:"website_url"`
	ContactEmail      string `json:"contact_email"`
	HeroVideoURL      string `json:"hero_video_url"`
}

// Every field is optional, but anything that becomes a link or a video on the
// site has to be an https address.
func (r *updateSettingsRequest) Validate() error {
	for name, value := range map[string]string{
		"photo": r.PastorPhotoURL, "books link": r.BooksURL,
		"website": r.WebsiteURL, "hero video": r.HeroVideoURL,
	} {
		if err := validate.HTTPSURL(name, value); err != nil {
			return err
		}
	}
	for name, value := range map[string]string{
		"name": r.PastorName, "campuses": r.CampusesCount, "years": r.YearsLeadership,
		"continents": r.ContinentsReached, "contact email": r.ContactEmail,
	} {
		if err := validate.MaxLen(name, value, 200); err != nil {
			return err
		}
	}
	return validate.MaxLen("bio", r.PastorBio, 4000)
}

func (h *SettingsHandler) AdminUpdate(c echo.Context) error {
	var req updateSettingsRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}

	settings, err := h.settings.Update(c.Request().Context(), service.UpdateSettingsInput{
		PastorName:        req.PastorName,
		PastorBio:         req.PastorBio,
		PastorPhotoURL:    req.PastorPhotoURL,
		CampusesCount:     req.CampusesCount,
		YearsLeadership:   req.YearsLeadership,
		ContinentsReached: req.ContinentsReached,
		BooksURL:          req.BooksURL,
		WebsiteURL:        req.WebsiteURL,
		ContactEmail:      req.ContactEmail,
		HeroVideoURL:      req.HeroVideoURL,
	})
	if err != nil {
		return response.Internal(c, "could not update site settings")
	}
	return response.OK(c, "settings updated", settings)
}

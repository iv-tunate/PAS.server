package handler

import (
	"errors"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/iv-tunate/PAS/internal/middleware"
	"github.com/iv-tunate/PAS/internal/service"
	"github.com/iv-tunate/PAS/pkg/response"
	"github.com/iv-tunate/PAS/pkg/validate"
)

// AuthHandler covers admin sign-in and admin account management. Visitors
// have no accounts, so there is no sign-up here.
type AuthHandler struct {
	auth *service.AuthService
}

func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

const minPasswordLen = 10

func checkPassword(p string) error {
	if len(p) < minPasswordLen {
		return errors.New("password must be at least 10 characters")
	}
	if len(p) > 72 { // bcrypt only reads the first 72 bytes
		return errors.New("password must be 72 characters or fewer")
	}
	return nil
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *loginRequest) Validate() error {
	return validate.Required(map[string]string{"email": r.Email, "password": r.Password})
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}

	user, tokens, err := h.auth.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		return response.Unauthorized(c, "invalid email or password")
	}

	return response.OK(c, "logged in", echo.Map{
		"user": echo.Map{
			"id":        user.ID,
			"full_name": user.FullName,
			"email":     user.Email,
			"role":      user.Role,
		},
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_in":    tokens.ExpiresIn,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (r *refreshRequest) Validate() error {
	return validate.Required(map[string]string{"refresh_token": r.RefreshToken})
}

func (h *AuthHandler) Refresh(c echo.Context) error {
	var req refreshRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}

	tokens, err := h.auth.Refresh(c.Request().Context(), req.RefreshToken)
	if err != nil {
		return response.Unauthorized(c, "session expired, please log in again")
	}

	return response.OK(c, "token refreshed", echo.Map{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_in":    tokens.ExpiresIn,
	})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	var req refreshRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	_ = h.auth.Logout(c.Request().Context(), req.RefreshToken)
	return response.OK(c, "logged out", nil)
}

// Me tells the dashboard who is signed in.
func (h *AuthHandler) Me(c echo.Context) error {
	claims := middleware.Claims(c)
	user, err := h.auth.GetByID(c.Request().Context(), claims.UserID)
	if err != nil {
		return response.Unauthorized(c, "account not found")
	}
	return response.OK(c, "", echo.Map{
		"id":        user.ID,
		"full_name": user.FullName,
		"email":     user.Email,
		"role":      user.Role,
	})
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

func (r *forgotPasswordRequest) Validate() error {
	return validate.Required(map[string]string{"email": r.Email})
}

// ForgotPassword always answers the same way, registered email or not, so it
// cannot be used to find out which emails belong to admins.
func (h *AuthHandler) ForgotPassword(c echo.Context, resetURLBase string) error {
	var req forgotPasswordRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}

	if err := h.auth.RequestPasswordReset(c.Request().Context(), req.Email, resetURLBase); err != nil {
		return response.Internal(c, "could not process request")
	}

	return response.OK(c, "if that email belongs to an admin, a reset link has been sent", nil)
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (r *resetPasswordRequest) Validate() error {
	if err := validate.Required(map[string]string{"token": r.Token, "new_password": r.NewPassword}); err != nil {
		return err
	}
	return checkPassword(r.NewPassword)
}

func (h *AuthHandler) ResetPassword(c echo.Context) error {
	var req resetPasswordRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}

	if err := h.auth.ResetPassword(c.Request().Context(), req.Token, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidToken) {
			return response.BadRequest(c, "invalid_token", "this reset link is invalid or has expired", nil)
		}
		return response.Internal(c, "could not reset password")
	}

	return response.OK(c, "password updated, please log in again", nil)
}

// ---------------------------------------------------------------------
// Admin accounts (admin-only routes)
// ---------------------------------------------------------------------

type adminView struct {
	ID        uuid.UUID `json:"id"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	CreatedAt string    `json:"created_at"`
}

func (h *AuthHandler) ListAdmins(c echo.Context) error {
	admins, err := h.auth.ListAdmins(c.Request().Context())
	if err != nil {
		return response.Internal(c, "could not load admins")
	}
	out := make([]adminView, 0, len(admins))
	for _, a := range admins {
		out = append(out, adminView{ID: a.ID, FullName: a.FullName, Email: a.Email, CreatedAt: a.CreatedAt.Time.Format("2006-01-02")})
	}
	return response.OK(c, "", out)
}

type createAdminRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *createAdminRequest) Validate() error {
	if err := validate.Email(r.Email); err != nil {
		return err
	}
	if err := validate.MaxLen("name", r.FullName, 100); err != nil {
		return err
	}
	return checkPassword(r.Password)
}

func (h *AuthHandler) CreateAdmin(c echo.Context) error {
	var req createAdminRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	user, err := h.auth.CreateAdmin(c.Request().Context(), req.FullName, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			return response.Conflict(c, "email_taken", err.Error())
		}
		return response.Internal(c, "could not create the admin")
	}
	return response.Created(c, "admin created", adminView{ID: user.ID, FullName: user.FullName, Email: user.Email, CreatedAt: user.CreatedAt.Time.Format("2006-01-02")})
}

func (h *AuthHandler) DeleteAdmin(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return response.BadRequest(c, "invalid_id", "invalid id", nil)
	}
	err = h.auth.DeleteAdmin(c.Request().Context(), middleware.Claims(c).UserID, id)
	switch {
	case errors.Is(err, service.ErrCannotDeleteSelf), errors.Is(err, service.ErrLastAdmin):
		return response.BadRequest(c, "not_allowed", err.Error(), nil)
	case err != nil:
		return response.Internal(c, "could not remove the admin")
	}
	return response.NoContent(c)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (r *changePasswordRequest) Validate() error {
	if err := validate.Required(map[string]string{"current_password": r.CurrentPassword, "new_password": r.NewPassword}); err != nil {
		return err
	}
	return checkPassword(r.NewPassword)
}

func (h *AuthHandler) ChangePassword(c echo.Context) error {
	var req changePasswordRequest
	if err := validate.BindAndValidate(c, &req); err != nil {
		return response.BadRequest(c, "invalid_request", err.Error(), nil)
	}
	err := h.auth.ChangePassword(c.Request().Context(), middleware.Claims(c).UserID, req.CurrentPassword, req.NewPassword)
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		return response.BadRequest(c, "wrong_password", "your current password is not right", nil)
	case err != nil:
		return response.Internal(c, "could not change the password")
	}
	return response.OK(c, "password changed, please log in again", nil)
}

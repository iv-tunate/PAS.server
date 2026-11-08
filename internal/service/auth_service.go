// Package service holds business logic. Handlers stay thin (parse
// request, call a service method, write a response); services own the
// rules, hashing, token generation, and orchestration across queries.
//
// Accounts exist for administrators only. Visitors never sign up or log in.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iv-tunate/PAS/internal/config"
	"github.com/iv-tunate/PAS/internal/db/sqlc"
	"github.com/iv-tunate/PAS/internal/mailer"
	"golang.org/x/crypto/bcrypt"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailTaken         = errors.New("an account with this email already exists")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrCannotDeleteSelf   = errors.New("you cannot remove your own account")
	ErrLastAdmin          = errors.New("there must always be at least one admin")
)

type AuthService struct {
	queries *sqlc.Queries
	cfg     *config.Config
	log     *slog.Logger
	outbox  *mailer.Outbox
}

func NewAuthService(pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger, outbox *mailer.Outbox) *AuthService {
	return &AuthService{queries: sqlc.New(pool), cfg: cfg, log: log, outbox: outbox}
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

const accessTokenTTL = 15 * time.Minute
const refreshTokenTTL = 7 * 24 * time.Hour

// CreateAdmin registers an administrator. It is the only way an account
// comes into existence: from the dashboard, or `go run . create-admin`.
func (s *AuthService) CreateAdmin(ctx context.Context, fullName, email, password string) (sqlc.User, error) {
	email = normalizeEmail(email)

	if _, err := s.queries.GetUserByEmail(ctx, email); err == nil {
		return sqlc.User{}, ErrEmailTaken
	}

	fullName = oneLine(fullName)
	if fullName == "" {
		fullName = email[:strings.IndexByte(email, '@')]
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return sqlc.User{}, err
	}

	user, err := s.queries.CreateUser(ctx, sqlc.CreateUserParams{
		FullName:     fullName,
		Email:        email,
		PasswordHash: string(hash),
		Role:         "admin",
	})
	if err != nil {
		return sqlc.User{}, err
	}

	s.log.Info("admin created", "user_id", user.ID, "email", user.Email)
	return user, nil
}

func (s *AuthService) ListAdmins(ctx context.Context) ([]sqlc.User, error) {
	return s.queries.ListAdmins(ctx)
}

func (s *AuthService) DeleteAdmin(ctx context.Context, actorID, targetID uuid.UUID) error {
	if actorID == targetID {
		return ErrCannotDeleteSelf
	}
	count, err := s.queries.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastAdmin
	}
	return s.queries.DeleteAdmin(ctx, targetID)
}

// ChangePassword needs the current password, so a borrowed, unlocked laptop
// is not enough to take over the account.
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, current, next string) error {
	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), 12)
	if err != nil {
		return err
	}
	if err := s.queries.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{ID: userID, PasswordHash: string(hash)}); err != nil {
		return err
	}
	return s.LogoutAllDevices(ctx, userID)
}

// A throwaway hash so a login for an email that does not exist takes as long
// as one that does. Otherwise response time would reveal which emails are admins.
var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

func burnPasswordCheck(password string) {
	dummyHashOnce.Do(func() { dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), 12) })
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

func (s *AuthService) Login(ctx context.Context, email, password string) (sqlc.User, TokenPair, error) {
	user, err := s.queries.GetUserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		burnPasswordCheck(password)
		return sqlc.User{}, TokenPair{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return sqlc.User{}, TokenPair{}, ErrInvalidCredentials
	}
	// Only admins can sign in. Same error as a wrong password on purpose.
	if user.Role != "admin" {
		return sqlc.User{}, TokenPair{}, ErrInvalidCredentials
	}

	pair, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return sqlc.User{}, TokenPair{}, err
	}

	return user, pair, nil
}

func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken string) (TokenPair, error) {
	hash := hashToken(rawRefreshToken)

	stored, err := s.queries.GetRefreshToken(ctx, hash)
	if err != nil {
		return TokenPair{}, ErrInvalidToken
	}

	user, err := s.queries.GetUserByID(ctx, stored.UserID)
	if err != nil || user.Role != "admin" {
		return TokenPair{}, ErrInvalidToken
	}

	if err := s.queries.RevokeRefreshToken(ctx, hash); err != nil {
		s.log.Error("failed to revoke rotated refresh token", "error", err)
	}

	return s.issueTokenPair(ctx, user)
}

func (s *AuthService) Logout(ctx context.Context, rawRefreshToken string) error {
	return s.queries.RevokeRefreshToken(ctx, hashToken(rawRefreshToken))
}

func (s *AuthService) GetByID(ctx context.Context, id uuid.UUID) (sqlc.User, error) {
	return s.queries.GetUserByID(ctx, id)
}

func (s *AuthService) LogoutAllDevices(ctx context.Context, userID uuid.UUID) error {
	return s.queries.RevokeAllUserRefreshTokens(ctx, userID)
}

const passwordResetTTL = 1 * time.Hour

func (s *AuthService) RequestPasswordReset(ctx context.Context, email, resetURLBase string) error {
	user, err := s.queries.GetUserByEmail(ctx, normalizeEmail(email))
	if err != nil || user.Role != "admin" {
		s.log.Info("password reset requested for an email that is not an admin", "email", normalizeEmail(email))
		return nil
	}

	rawToken, err := randomToken(32)
	if err != nil {
		return err
	}

	if _, err := s.queries.CreatePasswordResetToken(ctx, sqlc.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: pgTimestamp(time.Now().Add(passwordResetTTL)),
	}); err != nil {
		return err
	}

	link := resetURLBase + "?token=" + rawToken
	body := `<p style="margin:0 0 16px;line-height:1.65">Hi ` + esc(firstName(user.FullName)) + `,</p>` +
		`<p style="margin:0 0 16px;line-height:1.65">Use the button below to choose a new password. The link works for one hour.</p>` +
		emailButton(link, "Reset your password") +
		`<p style="margin:0;font-size:14px;color:#555">If you did not ask for this, you can ignore this email.</p>`

	if !s.outbox.TrySend(mailer.Job{Msg: mailer.Message{
		ToEmail: user.Email,
		ToName:  user.FullName,
		Subject: "Reset your password",
		HTML:    emailPage("Reset your password", body, ""),
	}}) {
		s.log.Error("could not queue password reset email", "user_id", user.ID)
	}

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash := hashToken(rawToken)

	stored, err := s.queries.GetValidPasswordResetToken(ctx, hash)
	if err != nil {
		return ErrInvalidToken
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}

	if err := s.queries.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{
		ID:           stored.UserID,
		PasswordHash: string(newHash),
	}); err != nil {
		return err
	}

	if err := s.queries.MarkPasswordResetTokenUsed(ctx, hash); err != nil {
		s.log.Error("failed to mark reset token used", "error", err)
	}

	return s.LogoutAllDevices(ctx, stored.UserID)
}

func (s *AuthService) issueTokenPair(ctx context.Context, user sqlc.User) (TokenPair, error) {
	access, err := s.signAccessToken(user)
	if err != nil {
		return TokenPair{}, err
	}

	rawRefresh, err := randomToken(32)
	if err != nil {
		return TokenPair{}, err
	}

	_, err = s.queries.CreateRefreshToken(ctx, sqlc.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: hashToken(rawRefresh),
		ExpiresAt: pgTimestamp(time.Now().Add(refreshTokenTTL)),
	})
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		ExpiresIn:    int64(accessTokenTTL.Seconds()),
	}, nil
}

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

func (s *AuthService) signAccessToken(user sqlc.User) (string, error) {
	claims := Claims{
		UserID: user.ID,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) ParseAccessToken(raw string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

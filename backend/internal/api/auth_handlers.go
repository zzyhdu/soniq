package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/auth"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

const maxAuthRequestBytes = 1 << 20

// PasswordAuthStore is the persistence seam for password signup and signin.
type PasswordAuthStore interface {
	SignUp(ctx context.Context, input auth.SignUpInput) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, string, bool, error)
}

// PasswordSessionStore is the persistence seam for issuing and revoking login sessions.
type PasswordSessionStore interface {
	CreateSession(ctx context.Context, input auth.CreateSessionInput) (auth.Session, error)
	RevokeSession(ctx context.Context, tokenHash string, now time.Time) error
}

// PasswordAuthConfig configures public email/password auth endpoints.
type PasswordAuthConfig struct {
	PasswordStore PasswordAuthStore
	SessionStore  PasswordSessionStore
	SessionTTL    time.Duration
	CookieName    string
	CookieSecure  bool
	Now           func() time.Time
}

type authUserResponse struct {
	User domain.User `json:"user"`
}

type signInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type signUpRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func signUpHandler(config PasswordAuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if config.PasswordStore == nil || config.SessionStore == nil {
			http.Error(w, "password auth is not configured", http.StatusInternalServerError)
			return
		}

		var request signUpRequest
		if !decodeAuthJSON(w, r, &request) {
			return
		}
		email := auth.NormalizeEmail(request.Email)
		displayName := strings.TrimSpace(request.DisplayName)
		if err := auth.ValidateEmail(email); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if displayName == "" {
			http.Error(w, "display name is required", http.StatusBadRequest)
			return
		}
		passwordHash, err := auth.HashPassword(request.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		user, err := config.PasswordStore.SignUp(r.Context(), auth.SignUpInput{
			Email:        email,
			DisplayName:  displayName,
			PasswordHash: passwordHash,
		})
		if err != nil {
			if errors.Is(err, auth.ErrUserAlreadyExists) {
				http.Error(w, "user already exists", http.StatusConflict)
				return
			}
			http.Error(w, "sign up user", http.StatusInternalServerError)
			return
		}
		if !issueSessionCookie(w, r, config, user.ID) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(authUserResponse{User: user})
	}
}

func signInHandler(config PasswordAuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if config.PasswordStore == nil || config.SessionStore == nil {
			http.Error(w, "password auth is not configured", http.StatusInternalServerError)
			return
		}

		var request signInRequest
		if !decodeAuthJSON(w, r, &request) {
			return
		}
		email := auth.NormalizeEmail(request.Email)
		if err := auth.ValidateEmail(email); err != nil {
			http.Error(w, "invalid email or password", http.StatusUnauthorized)
			return
		}
		user, passwordHash, found, err := config.PasswordStore.GetUserByEmail(r.Context(), email)
		if err != nil {
			http.Error(w, "get user by email", http.StatusInternalServerError)
			return
		}
		if !found || !auth.VerifyPassword(passwordHash, request.Password) {
			http.Error(w, "invalid email or password", http.StatusUnauthorized)
			return
		}
		if !issueSessionCookie(w, r, config, user.ID) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(authUserResponse{User: user})
	}
}

func signOutHandler(config PasswordAuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cookieName := authCookieName(config)
		if cookie, err := r.Cookie(cookieName); err == nil && strings.TrimSpace(cookie.Value) != "" && config.SessionStore != nil {
			if err := config.SessionStore.RevokeSession(r.Context(), auth.HashSessionToken(cookie.Value), authNow(config)); err != nil {
				http.Error(w, "revoke session", http.StatusInternalServerError)
				return
			}
		}
		clearSessionCookie(w, config)
		w.WriteHeader(http.StatusNoContent)
	}
}

func decodeAuthJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(destination); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return false
	}
	return true
}

func issueSessionCookie(w http.ResponseWriter, r *http.Request, config PasswordAuthConfig, userID string) bool {
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		http.Error(w, "create session token", http.StatusInternalServerError)
		return false
	}
	expiresAt := authNow(config).Add(authSessionTTL(config))
	if _, err := config.SessionStore.CreateSession(r.Context(), auth.CreateSessionInput{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}); err != nil {
		http.Error(w, "create session", http.StatusInternalServerError)
		return false
	}
	setSessionCookie(w, config, token, expiresAt)
	return true
}

func setSessionCookie(w http.ResponseWriter, config PasswordAuthConfig, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName(config),
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(authSessionTTL(config).Seconds()),
		HttpOnly: true,
		Secure:   config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, config PasswordAuthConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName(config),
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func authCookieName(config PasswordAuthConfig) string {
	cookieName := strings.TrimSpace(config.CookieName)
	if cookieName == "" {
		return DefaultSessionCookieName
	}
	return cookieName
}

func authSessionTTL(config PasswordAuthConfig) time.Duration {
	if config.SessionTTL <= 0 {
		return 30 * 24 * time.Hour
	}
	return config.SessionTTL
}

func authNow(config PasswordAuthConfig) time.Time {
	if config.Now != nil {
		return config.Now().UTC()
	}
	return time.Now().UTC()
}

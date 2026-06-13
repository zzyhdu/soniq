package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/auth"
	"github.com/zzyhdu/soniq/backend/internal/domain"
)

func TestSignUpCreatesUserAndSessionCookie(t *testing.T) {
	authStore := newPasswordAuthStoreSpy()
	router := NewRouterWithStorageIdentityAndPasswordAuth(unconfiguredRecordingStore{}, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), nil, nil, passwordAuthTestConfig(authStore))
	request := httptest.NewRequest(http.MethodPost, "/auth/signup", strings.NewReader(`{"email":"Owner@Local.Soniq","display_name":"Owner","password":"correct horse"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	if authStore.signUp.Email != "owner@local.soniq" {
		t.Fatalf("signup email = %q, want normalized owner email", authStore.signUp.Email)
	}
	if !auth.VerifyPassword(authStore.signUp.PasswordHash, "correct horse") {
		t.Fatal("signup password hash does not verify password")
	}
	assertSessionCookie(t, response.Result(), "soniq_test")
	if got, want := len(authStore.sessions), 1; got != want {
		t.Fatalf("created sessions = %d, want %d", got, want)
	}
}

func TestSignInCreatesSessionCookie(t *testing.T) {
	authStore := newPasswordAuthStoreSpy()
	passwordHash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	authStore.passwordHash = passwordHash
	router := NewRouterWithStorageIdentityAndPasswordAuth(unconfiguredRecordingStore{}, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), nil, nil, passwordAuthTestConfig(authStore))
	request := httptest.NewRequest(http.MethodPost, "/auth/signin", strings.NewReader(`{"email":"owner@local.soniq","password":"correct horse"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body authUserResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.User.ID != "usr_dev" {
		t.Fatalf("user id = %q, want usr_dev", body.User.ID)
	}
	assertSessionCookie(t, response.Result(), "soniq_test")
	if got, want := len(authStore.sessions), 1; got != want {
		t.Fatalf("created sessions = %d, want %d", got, want)
	}
	if authStore.sessions[0].TokenHash == "" {
		t.Fatal("session token hash is empty")
	}
}

func TestSignInRejectsInvalidPassword(t *testing.T) {
	authStore := newPasswordAuthStoreSpy()
	passwordHash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	authStore.passwordHash = passwordHash
	router := NewRouterWithStorageIdentityAndPasswordAuth(unconfiguredRecordingStore{}, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), nil, nil, passwordAuthTestConfig(authStore))
	request := httptest.NewRequest(http.MethodPost, "/auth/signin", strings.NewReader(`{"email":"owner@local.soniq","password":"wrong horse"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	var body apiErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Code != errorCodeInvalidCredentials || body.Status != http.StatusUnauthorized {
		t.Fatalf("error body = %+v, want invalid_credentials 401", body)
	}
	if got, want := len(authStore.sessions), 0; got != want {
		t.Fatalf("created sessions = %d, want %d", got, want)
	}
}

func TestSignOutRevokesSessionAndClearsCookie(t *testing.T) {
	authStore := newPasswordAuthStoreSpy()
	router := NewRouterWithStorageIdentityAndPasswordAuth(unconfiguredRecordingStore{}, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), nil, nil, passwordAuthTestConfig(authStore))
	request := httptest.NewRequest(http.MethodPost, "/auth/signout", nil)
	request.AddCookie(&http.Cookie{Name: "soniq_test", Value: "opaque-token"})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusNoContent)
	}
	if authStore.revokedTokenHash != auth.HashSessionToken("opaque-token") {
		t.Fatal("logout did not revoke hashed session token")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "soniq_test" || cookies[0].MaxAge != -1 {
		t.Fatalf("clear cookie = %+v, want expired soniq_test cookie", cookies)
	}
}

func TestSignOutClearsCookieWhenSessionRevokeFails(t *testing.T) {
	authStore := newPasswordAuthStoreSpy()
	authStore.revokeErr = errors.New("database unavailable")
	router := NewRouterWithStorageIdentityAndPasswordAuth(unconfiguredRecordingStore{}, defaultDevWorkspaceStore{}, NewDevAuthResolver("usr_dev"), nil, nil, passwordAuthTestConfig(authStore))
	request := httptest.NewRequest(http.MethodPost, "/auth/signout", nil)
	request.AddCookie(&http.Cookie{Name: "soniq_test", Value: "opaque-token"})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "soniq_test" || cookies[0].MaxAge != -1 {
		t.Fatalf("clear cookie = %+v, want expired soniq_test cookie", cookies)
	}
}

func passwordAuthTestConfig(store *passwordAuthStoreSpy) PasswordAuthConfig {
	return PasswordAuthConfig{
		PasswordStore: store,
		SessionStore:  store,
		SessionTTL:    time.Hour,
		CookieName:    "soniq_test",
		Now: func() time.Time {
			return time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
		},
	}
}

func assertSessionCookie(t *testing.T, response *http.Response, cookieName string) {
	t.Helper()

	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != cookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, cookieName)
	}
	if cookie.Value == "" {
		t.Fatal("cookie value is empty")
	}
	if !cookie.HttpOnly {
		t.Fatal("cookie HttpOnly = false, want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite = %d, want lax", cookie.SameSite)
	}
}

type passwordAuthStoreSpy struct {
	user             domain.User
	passwordHash     string
	signUp           auth.SignUpInput
	sessions         []auth.CreateSessionInput
	revokedTokenHash string
	revokeErr        error
}

func newPasswordAuthStoreSpy() *passwordAuthStoreSpy {
	return &passwordAuthStoreSpy{
		user: domain.User{
			ID:          "usr_dev",
			Email:       "owner@local.soniq",
			DisplayName: "Owner",
		},
	}
}

func (s *passwordAuthStoreSpy) SignUp(_ context.Context, input auth.SignUpInput) (domain.User, error) {
	if input.Email == s.user.Email && s.passwordHash != "" {
		return domain.User{}, auth.ErrUserAlreadyExists
	}
	s.signUp = input
	s.user.Email = input.Email
	s.user.DisplayName = input.DisplayName
	s.passwordHash = input.PasswordHash
	return s.user, nil
}

func (s *passwordAuthStoreSpy) GetUserByEmail(_ context.Context, email string) (domain.User, string, bool, error) {
	if email != s.user.Email {
		return domain.User{}, "", false, nil
	}
	return s.user, s.passwordHash, true, nil
}

func (s *passwordAuthStoreSpy) CreateSession(_ context.Context, input auth.CreateSessionInput) (auth.Session, error) {
	s.sessions = append(s.sessions, input)
	return auth.Session{ID: "ses_test", UserID: input.UserID, TokenHash: input.TokenHash, ExpiresAt: input.ExpiresAt}, nil
}

func (s *passwordAuthStoreSpy) RevokeSession(_ context.Context, tokenHash string, _ time.Time) error {
	s.revokedTokenHash = tokenHash
	return s.revokeErr
}

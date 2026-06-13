package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/auth"
)

func TestDevAuthResolverDefaultsToDevUser(t *testing.T) {
	resolver := NewDevAuthResolver("")

	user, err := resolver.ResolveCurrentUser(httptest.NewRequest("GET", "/me", nil))
	if err != nil {
		t.Fatalf("ResolveCurrentUser returned error: %v", err)
	}
	if user.UserID != "usr_dev" {
		t.Fatalf("UserID = %q, want usr_dev", user.UserID)
	}
}

func TestDevAuthResolverReturnsConfiguredUser(t *testing.T) {
	resolver := NewDevAuthResolver("usr_custom")

	user, err := resolver.ResolveCurrentUser(httptest.NewRequest("GET", "/me", nil))
	if err != nil {
		t.Fatalf("ResolveCurrentUser returned error: %v", err)
	}
	if user.UserID != "usr_custom" {
		t.Fatalf("UserID = %q, want usr_custom", user.UserID)
	}
}

func TestSessionAuthResolverReturnsSessionUser(t *testing.T) {
	now := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	store := &sessionLookupStoreSpy{
		session: auth.Session{UserID: "usr_owner"},
		found:   true,
	}
	resolver := SessionAuthResolver{Store: store, CookieName: "soniq_test", Now: func() time.Time { return now }}
	request := httptest.NewRequest("GET", "/me", nil)
	request.AddCookie(&http.Cookie{Name: "soniq_test", Value: "opaque-token"})

	user, err := resolver.ResolveCurrentUser(request)
	if err != nil {
		t.Fatalf("ResolveCurrentUser returned error: %v", err)
	}
	if user.UserID != "usr_owner" {
		t.Fatalf("UserID = %q, want usr_owner", user.UserID)
	}
	if store.tokenHash != auth.HashSessionToken("opaque-token") {
		t.Fatal("session lookup did not receive hashed token")
	}
	if !store.now.Equal(now) {
		t.Fatalf("lookup now = %s, want %s", store.now, now)
	}
}

func TestSessionAuthResolverRejectsMissingCookie(t *testing.T) {
	resolver := NewSessionAuthResolver(&sessionLookupStoreSpy{})

	if _, err := resolver.ResolveCurrentUser(httptest.NewRequest("GET", "/me", nil)); err == nil {
		t.Fatal("ResolveCurrentUser error = nil, want missing cookie error")
	}
}

type sessionLookupStoreSpy struct {
	tokenHash string
	now       time.Time
	session   auth.Session
	found     bool
}

func (s *sessionLookupStoreSpy) GetActiveSessionByTokenHash(_ context.Context, tokenHash string, now time.Time) (auth.Session, bool, error) {
	s.tokenHash = tokenHash
	s.now = now
	return s.session, s.found, nil
}

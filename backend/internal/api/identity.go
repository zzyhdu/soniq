package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zzyhdu/soniq/backend/internal/auth"
)

const DefaultSessionCookieName = "soniq_session"

// CurrentUser is the authenticated user resolved for a request.
type CurrentUser struct {
	UserID string
}

// AuthResolver resolves the current user from a request.
type AuthResolver interface {
	ResolveCurrentUser(r *http.Request) (CurrentUser, error)
}

// DevAuthResolver resolves every request to a configured local-development user.
type DevAuthResolver struct {
	UserID string
}

// NewDevAuthResolver creates the local development auth resolver.
func NewDevAuthResolver(userID string) DevAuthResolver {
	if strings.TrimSpace(userID) == "" {
		userID = "usr_dev"
	}
	return DevAuthResolver{UserID: userID}
}

// ResolveCurrentUser returns the configured dev user for every request.
func (r DevAuthResolver) ResolveCurrentUser(_ *http.Request) (CurrentUser, error) {
	userID := strings.TrimSpace(r.UserID)
	if userID == "" {
		userID = "usr_dev"
	}
	return CurrentUser{UserID: userID}, nil
}

// SessionLookupStore is the persistence seam required by cookie session auth.
type SessionLookupStore interface {
	GetActiveSessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (auth.Session, bool, error)
}

// SessionAuthResolver resolves current users from an httpOnly session cookie.
type SessionAuthResolver struct {
	Store      SessionLookupStore
	CookieName string
	Now        func() time.Time
}

// NewSessionAuthResolver creates the password-auth session resolver.
func NewSessionAuthResolver(store SessionLookupStore) SessionAuthResolver {
	return SessionAuthResolver{Store: store, CookieName: DefaultSessionCookieName}
}

// ResolveCurrentUser returns the session owner when the request has an active session cookie.
func (r SessionAuthResolver) ResolveCurrentUser(request *http.Request) (CurrentUser, error) {
	if request == nil {
		return CurrentUser{}, fmt.Errorf("request is required")
	}
	if r.Store == nil {
		return CurrentUser{}, fmt.Errorf("session store is required")
	}
	cookieName := strings.TrimSpace(r.CookieName)
	if cookieName == "" {
		cookieName = DefaultSessionCookieName
	}
	cookie, err := request.Cookie(cookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return CurrentUser{}, fmt.Errorf("session cookie is required")
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	session, ok, err := r.Store.GetActiveSessionByTokenHash(request.Context(), auth.HashSessionToken(cookie.Value), now().UTC())
	if err != nil {
		return CurrentUser{}, err
	}
	if !ok || strings.TrimSpace(session.UserID) == "" {
		return CurrentUser{}, fmt.Errorf("session is invalid")
	}
	return CurrentUser{UserID: session.UserID}, nil
}

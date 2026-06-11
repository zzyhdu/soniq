package api

import (
	"net/http"
	"strings"
)

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

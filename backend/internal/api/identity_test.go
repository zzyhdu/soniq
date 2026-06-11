package api

import (
	"net/http/httptest"
	"testing"
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

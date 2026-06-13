package auth

import "testing"

func TestNewSessionTokenReturnsURLSafeTokenAndHash(t *testing.T) {
	token, tokenHash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}
	if tokenHash == "" {
		t.Fatal("tokenHash is empty")
	}
	if token == tokenHash {
		t.Fatal("tokenHash should not equal token")
	}
	if HashSessionToken(token) != tokenHash {
		t.Fatal("HashSessionToken(token) did not match generated token hash")
	}
}

func TestNewSessionIDUsesSessionPrefix(t *testing.T) {
	id, err := NewSessionID()
	if err != nil {
		t.Fatalf("NewSessionID returned error: %v", err)
	}
	if len(id) <= len("ses_") || id[:4] != "ses_" {
		t.Fatalf("session id = %q, want ses_ prefix", id)
	}
}

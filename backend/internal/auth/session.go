package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	userIDRandomBytes      = 16
	workspaceIDRandomBytes = 16
	sessionIDRandomBytes   = 16
	sessionTokenBytes      = 32
)

// Session is a persisted login session. Only TokenHash is stored in Postgres.
type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

// CreateSessionInput contains the values needed to persist a new login session.
type CreateSessionInput struct {
	UserID    string
	TokenHash string
	ExpiresAt time.Time
}

// NewSessionID returns a URL-safe, non-secret session row id.
func NewSessionID() (string, error) {
	random, err := randomBytes(sessionIDRandomBytes)
	if err != nil {
		return "", err
	}
	return "ses_" + hex.EncodeToString(random), nil
}

// NewUserID returns a URL-safe product user id.
func NewUserID() (string, error) {
	random, err := randomBytes(userIDRandomBytes)
	if err != nil {
		return "", err
	}
	return "usr_" + hex.EncodeToString(random), nil
}

// NewWorkspaceID returns a URL-safe workspace id.
func NewWorkspaceID() (string, error) {
	random, err := randomBytes(workspaceIDRandomBytes)
	if err != nil {
		return "", err
	}
	return "wsp_" + hex.EncodeToString(random), nil
}

// NewSessionToken returns an opaque bearer token and the hash that should be stored.
func NewSessionToken() (string, string, error) {
	random, err := randomBytes(sessionTokenBytes)
	if err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	return token, HashSessionToken(token), nil
}

// HashSessionToken returns the stable hash stored in Postgres for an opaque cookie token.
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func randomBytes(size int) ([]byte, error) {
	random := make([]byte, size)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("generate random bytes: %w", err)
	}
	return random, nil
}

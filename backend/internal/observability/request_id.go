package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	RequestIDHeader = "X-Request-ID"

	requestIDBytes     = 16
	maxRequestIDLength = 128
)

type requestIDContextKey struct{}

// NewRequestID returns an opaque request identifier safe to log and return.
func NewRequestID() string {
	var bytes [requestIDBytes]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "req_" + hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("req_%d", time.Now().UTC().UnixNano())
}

// RequestIDFromHeader normalizes a caller-provided request ID.
func RequestIDFromHeader(raw string) (string, bool) {
	requestID := strings.TrimSpace(raw)
	if requestID == "" || len(requestID) > maxRequestIDLength {
		return "", false
	}
	for _, character := range requestID {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return "", false
		}
	}
	return requestID, true
}

// ContextWithRequestID stores a request ID in context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

// RequestIDFromContext returns the request ID stored in context.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(requestIDContextKey{}).(string)
	if !ok || strings.TrimSpace(requestID) == "" {
		return "", false
	}
	return requestID, true
}

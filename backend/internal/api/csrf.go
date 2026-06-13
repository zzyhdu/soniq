package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

const (
	// CSRFHeaderName is the request header used by browser clients for unsafe methods.
	CSRFHeaderName        = "X-CSRF-Token"
	DefaultCSRFCookieName = "soniq_csrf"

	csrfTokenVersion    = "v1"
	csrfTokenNonceBytes = 32
	maxCSRFTokenLength  = 256
)

func csrfProtectionMiddleware(config PasswordAuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeHTTPMethod(r.Method) {
				ensureCSRFCookie(w, r, config)
				next.ServeHTTP(w, r)
				return
			}
			if isCSRFExemptPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			sessionCookie, err := r.Cookie(authCookieName(config))
			if err != nil || strings.TrimSpace(sessionCookie.Value) == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !validCSRFRequest(r, config, sessionCookie.Value) {
				writeAPIError(w, http.StatusForbidden, errorCodeInvalidCSRFToken, "invalid csrf token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request, config PasswordAuthConfig) {
	sessionCookie, err := r.Cookie(authCookieName(config))
	if err != nil || strings.TrimSpace(sessionCookie.Value) == "" {
		return
	}
	if csrfCookie, err := r.Cookie(csrfCookieName(config)); err == nil && validCSRFToken(sessionCookie.Value, csrfCookie.Value) {
		return
	}

	token, err := newCSRFToken(sessionCookie.Value)
	if err != nil {
		return
	}
	setCSRFCookie(w, config, token, authNow(config).Add(authSessionTTL(config)))
}

func validCSRFRequest(r *http.Request, config PasswordAuthConfig, sessionToken string) bool {
	headerToken := strings.TrimSpace(r.Header.Get(CSRFHeaderName))
	if headerToken == "" {
		return false
	}
	csrfCookie, err := r.Cookie(csrfCookieName(config))
	if err != nil || strings.TrimSpace(csrfCookie.Value) == "" {
		return false
	}
	if headerToken != csrfCookie.Value {
		return false
	}
	return validCSRFToken(sessionToken, headerToken)
}

func newCSRFToken(sessionToken string) (string, error) {
	nonce := make([]byte, csrfTokenNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	encodedNonce := base64.RawURLEncoding.EncodeToString(nonce)
	return csrfTokenVersion + "." + encodedNonce + "." + csrfTokenSignature(sessionToken, encodedNonce), nil
}

func validCSRFToken(sessionToken string, token string) bool {
	if strings.TrimSpace(sessionToken) == "" || len(token) > maxCSRFTokenLength {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != csrfTokenVersion || parts[1] == "" || parts[2] == "" {
		return false
	}
	expected := csrfTokenSignature(sessionToken, parts[1])
	return hmac.Equal([]byte(expected), []byte(parts[2]))
}

func csrfTokenSignature(sessionToken string, encodedNonce string) string {
	mac := hmac.New(sha256.New, []byte(sessionToken))
	_, _ = mac.Write([]byte(csrfTokenVersion))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write([]byte(encodedNonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func setCSRFCookie(w http.ResponseWriter, config PasswordAuthConfig, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName(config),
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(authSessionTTL(config).Seconds()),
		HttpOnly: false,
		Secure:   config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCSRFCookie(w http.ResponseWriter, config PasswordAuthConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName(config),
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func csrfCookieName(config PasswordAuthConfig) string {
	cookieName := strings.TrimSpace(config.CSRFCookieName)
	if cookieName == "" {
		return DefaultCSRFCookieName
	}
	return cookieName
}

func isSafeHTTPMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace
}

func isCSRFExemptPath(path string) bool {
	return path == "/auth/signup" || path == "/auth/signin"
}

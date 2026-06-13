package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthRateLimitWindow = 5 * time.Minute
	defaultSignInAuthRateLimit = 10
	defaultSignUpAuthRateLimit = 5
	authRateLimitActionSignIn  = AuthRateLimitAction("signin")
	authRateLimitActionSignUp  = AuthRateLimitAction("signup")
	unknownAuthRateLimitPart   = "unknown"
)

// AuthRateLimitAction identifies the auth operation being rate limited.
type AuthRateLimitAction string

// AuthRateLimiter decides whether an auth attempt should be allowed.
type AuthRateLimiter interface {
	AllowAuthAttempt(now time.Time, action AuthRateLimitAction, clientIP string, email string) bool
}

// AuthRateLimitConfig configures the in-memory auth rate limiter.
type AuthRateLimitConfig struct {
	Window      time.Duration
	SignInLimit int
	SignUpLimit int
}

// InMemoryAuthRateLimiter rate-limits auth attempts in the current API process.
type InMemoryAuthRateLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	lastCleanup time.Time
	limits      map[AuthRateLimitAction]int
	buckets     map[authRateLimitKey]authRateLimitBucket
}

type authRateLimitKey struct {
	action   AuthRateLimitAction
	clientIP string
	email    string
}

type authRateLimitBucket struct {
	windowStart time.Time
	count       int
}

// NewInMemoryAuthRateLimiter creates a fixed-window auth limiter.
func NewInMemoryAuthRateLimiter(config AuthRateLimitConfig) *InMemoryAuthRateLimiter {
	window := config.Window
	if window <= 0 {
		window = defaultAuthRateLimitWindow
	}
	signInLimit := config.SignInLimit
	if signInLimit <= 0 {
		signInLimit = defaultSignInAuthRateLimit
	}
	signUpLimit := config.SignUpLimit
	if signUpLimit <= 0 {
		signUpLimit = defaultSignUpAuthRateLimit
	}
	return &InMemoryAuthRateLimiter{
		window: window,
		limits: map[AuthRateLimitAction]int{
			authRateLimitActionSignIn: signInLimit,
			authRateLimitActionSignUp: signUpLimit,
		},
		buckets: make(map[authRateLimitKey]authRateLimitBucket),
	}
}

// AllowAuthAttempt returns false when the current fixed window is exhausted.
func (l *InMemoryAuthRateLimiter) AllowAuthAttempt(now time.Time, action AuthRateLimitAction, clientIP string, email string) bool {
	if l == nil {
		return true
	}
	limit := l.limits[action]
	if limit <= 0 {
		return true
	}
	key := authRateLimitKey{
		action:   action,
		clientIP: normalizeAuthRateLimitPart(clientIP),
		email:    normalizeAuthRateLimitPart(email),
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanupExpiredBucketsLocked(now)
	bucket := l.buckets[key]
	if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= l.window {
		bucket = authRateLimitBucket{windowStart: now}
	}
	if bucket.count >= limit {
		l.buckets[key] = bucket
		return false
	}
	bucket.count++
	l.buckets[key] = bucket
	return true
}

func (l *InMemoryAuthRateLimiter) cleanupExpiredBucketsLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < l.window {
		return
	}
	for key, bucket := range l.buckets {
		if now.Sub(bucket.windowStart) >= l.window {
			delete(l.buckets, key)
		}
	}
	l.lastCleanup = now
}

func allowAuthAttempt(w http.ResponseWriter, r *http.Request, config PasswordAuthConfig, action AuthRateLimitAction, email string) bool {
	if config.RateLimiter == nil {
		return true
	}
	if config.RateLimiter.AllowAuthAttempt(authNow(config), action, authClientIP(r), email) {
		return true
	}
	writeAPIError(w, http.StatusTooManyRequests, errorCodeRateLimited, "too many requests")
	return false
}

func authClientIP(r *http.Request) string {
	if r == nil {
		return unknownAuthRateLimitPart
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return unknownAuthRateLimitPart
}

func normalizeAuthRateLimitPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return unknownAuthRateLimitPart
	}
	return value
}

package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "dbvault_session"

	// loginWindow bounds the fixed-window rate limit per remote IP.
	loginWindow   = time.Minute
	loginMaxTries = 10
)

type principalKey struct{}

// Middleware enforces authentication on protected paths and attaches the
// resolved Principal to the request context.
type Middleware struct {
	store       *Store
	metricsPath string
	logger      *slog.Logger

	rlMu     sync.Mutex
	attempts map[string]*window
}

// NewMiddleware builds middleware over the given store.
func NewMiddleware(store *Store) *Middleware {
	return &Middleware{store: store, attempts: map[string]*window{}}
}

// SetLogger attaches a structured logger for security-relevant warnings.
func (m *Middleware) SetLogger(l *slog.Logger) {
	if l != nil {
		m.logger = l
	}
}

// SetMetricsPath exempts the metrics endpoint from auth (Phase C mounts it;
// access control belongs at the TLS-terminating reverse proxy).
func (m *Middleware) SetMetricsPath(path string) {
	if path != "" {
		m.metricsPath = path
	}
}

func (m *Middleware) Store() *Store { return m.store }

type window struct {
	start time.Time
	count int
}

// allow consumes one attempt for ip under a fixed window; fail-closed.
func (m *Middleware) allow(ip string, now time.Time) bool {
	if ip == "" {
		return false
	}
	m.rlMu.Lock()
	defer m.rlMu.Unlock()
	w, ok := m.attempts[ip]
	if !ok || now.Sub(w.start) >= loginWindow {
		m.attempts[ip] = &window{start: now, count: 1}
		if len(m.attempts) > 10_000 {
			for k, v := range m.attempts {
				if now.Sub(v.start) >= loginWindow {
					delete(m.attempts, k)
				}
			}
		}
		return true
	}
	w.count++
	return w.count <= loginMaxTries
}

// Exempt reports whether the request path bypasses authentication.
func (m *Middleware) Exempt(path string) bool {
	switch path {
	case "/health", "/ready":
		return true
	case "/api/v1/auth/login", "/api/v1/auth/bootstrap", "/api/v1/auth/tokens":
		return true // credential-presenting endpoints; rate limited instead
	}
	if strings.HasPrefix(path, "/api/v1/setup") {
		return true
	}
	if m.metricsPath != "" && path == m.metricsPath {
		return true
	}
	return false
}

// Protected reports whether the path falls inside the enforced surface.
func Protected(path string) bool {
	const prefix = "/api/v1"
	return path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"?")
}

// Protect wraps next with the trust boundary.
func (m *Middleware) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Protected(r.URL.Path) || m.Exempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := m.authenticate(r)
		if !ok {
			w.Header().Set("Cache-Control", "no-store")
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)))
	})
}

func (m *Middleware) authenticate(r *http.Request) (Principal, bool) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if p, valid := m.store.AuthenticateSession(c.Value); valid {
			return p, true
		}
	}
	authz := r.Header.Get("Authorization")
	if token, found := strings.CutPrefix(authz, "Bearer "); found && token != "" {
		if p, valid := m.store.AuthenticateToken(token); valid {
			return p, true
		}
	}
	return Principal{}, false
}

// PrincipalFromContext returns the authenticated principal, if any.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// RequireAdmin is a handler wrapper for admin-only endpoints such as master
// key operations. Unauthenticated callers receive 401 from Protect already;
// this guards against direct wiring mistakes by re-checking the context.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		if !ok || p.Role != RoleAdmin {
			writeJSONError(w, http.StatusForbidden, "administrator role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func attemptKey(r *http.Request) string {
	ip := clientIP(r)
	if ip == "" {
		return ""
	}
	return ip + "|" + r.URL.Path
}

func (m *Middleware) admitLoginAttempt(r *http.Request) bool {
	return m.allow(attemptKey(r), time.Now().UTC())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status})
}

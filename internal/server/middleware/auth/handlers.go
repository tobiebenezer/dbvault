package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// RegisterPublic mounts the unauthenticated (but rate-limited) credential
// endpoints: admin bootstrap through the setup token, login, and bearer-token
// issuance. setupTokenVerify checks the one-time token; consumeSetupToken is
// called only after the administrator has been created successfully.
func (m *Middleware) RegisterPublic(mux *http.ServeMux, setupTokenVerify func(token string) error, consumeSetupToken func() error) {
	mux.HandleFunc("POST /api/v1/auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if !m.admitLoginAttempt(r) {
			writeJSONError(w, http.StatusTooManyRequests, "too many attempts; retry later")
			return
		}
		var req struct {
			SetupToken string `json:"setup_token"`
			Username   string `json:"username"`
			Password   string `json:"password"`
		}
		if err := decodeBody(w, r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.SetupToken == "" {
			writeJSONError(w, http.StatusBadRequest, "setup_token is required")
			return
		}
		if m.store.Bootstrapped() {
			writeJSONError(w, http.StatusForbidden, "setup already completed")
			return
		}
		if setupTokenVerify != nil {
			if err := setupTokenVerify(req.SetupToken); err != nil {
				writeJSONError(w, http.StatusForbidden, "invalid setup token")
				return
			}
		}
		if err := m.store.BootstrapAdmin(req.Username, req.Password); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if consumeSetupToken != nil {
			if err := consumeSetupToken(); err != nil && m.logger != nil {
				m.logger.Warn("auth.setup_token_consume_failed", "error", err.Error())
			}
		}
		sessionID, expires, err := m.store.Login(req.Username, req.Password)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "administrator created but session failed")
			return
		}
		setSessionCookie(w, sessionID, expires)
		writeJSON(w, http.StatusCreated, map[string]any{"status": "administrator-created", "username": strings.TrimSpace(req.Username)})
	})

	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if !m.admitLoginAttempt(r) {
			writeJSONError(w, http.StatusTooManyRequests, "too many attempts; retry later")
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeBody(w, r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		sessionID, expires, err := m.store.Login(req.Username, req.Password)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		setSessionCookie(w, sessionID, expires)
		writeJSON(w, http.StatusOK, map[string]any{"status": "authenticated", "expires_at": expires})
	})

	mux.HandleFunc("POST /api/v1/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		if !m.admitLoginAttempt(r) {
			writeJSONError(w, http.StatusTooManyRequests, "too many attempts; retry later")
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeBody(w, r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		token, expires, err := m.store.IssueToken(req.Username, req.Password)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]any{"token": token, "token_type": "bearer", "expires_at": expires})
	})
}

// RegisterProtected mounts endpoints that require an authenticated principal.
func (m *Middleware) RegisterProtected(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			m.store.Logout(c.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged-out"})
	})

	mux.HandleFunc("GET /api/v1/auth/session", func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFromContext(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"subject": p.Subject, "role": p.Role, "via": p.Via})
	})
}

func setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) || err == nil {
			return errors.New("invalid request body")
		}
		return err
	}
	return nil
}

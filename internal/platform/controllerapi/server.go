package controllerapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dbvault/dbvault/internal/application/controlplane"
	"github.com/dbvault/dbvault/internal/domain"
)

type Server struct {
	Store *controlplane.Store
	Token string
	Now   func() time.Time
}

func New(store *controlplane.Store, token string) *Server {
	if store == nil {
		store = controlplane.NewStore()
	}
	return &Server{Store: store, Token: token, Now: func() time.Time { return time.Now().UTC() }}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "healthy"})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})
	mux.Handle("POST /v1/organisations", s.auth(http.HandlerFunc(s.createOrganisation)))
	mux.Handle("POST /v1/projects", s.auth(http.HandlerFunc(s.createProject)))
	mux.Handle("GET /v1/sources", s.auth(http.HandlerFunc(s.listSources)))
	mux.Handle("POST /v1/sources", s.auth(http.HandlerFunc(s.putSource)))
	mux.Handle("POST /v1/agents/register", s.auth(http.HandlerFunc(s.registerAgent)))
	mux.Handle("POST /v1/agents/heartbeat", s.auth(http.HandlerFunc(s.heartbeat)))
	return securityHeaders(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Token != "" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if len(provided) != len(s.Token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.Token)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorised")
				return
			}
		}
		scope, err := scopeFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), scopeKey{}, scope)))
	})
}

func (s *Server) createOrganisation(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())
	var org domain.Organisation
	if err := decodeJSON(r, &org); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if org.ID == "" {
		org.ID = scope.OrganisationID
	}
	if org.ID != scope.OrganisationID {
		writeError(w, http.StatusForbidden, "cannot create an organisation outside the caller scope")
		return
	}
	if org.CreatedAt.IsZero() {
		org.CreatedAt, org.UpdatedAt = s.Now(), s.Now()
	}
	if org.Status == "" {
		org.Status = domain.OrganisationActive
	}
	if err := s.Store.CreateOrganisation(org); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())
	var project domain.Project
	if err := decodeJSON(r, &project); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	project.OrganisationID = scope.OrganisationID
	if project.CreatedAt.IsZero() {
		project.CreatedAt, project.UpdatedAt = s.Now(), s.Now()
	}
	if err := s.Store.CreateProject(scope, project); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.ListSources(scopeFromContext(r.Context())))
}

func (s *Server) putSource(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())
	var source controlplane.ManagedSource
	if err := decodeJSON(r, &source); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source.OrganisationID, source.ProjectID = scope.OrganisationID, scope.ProjectID
	if scope.EnvironmentID != "" {
		source.EnvironmentID = scope.EnvironmentID
	}
	if source.CreatedAt.IsZero() {
		source.CreatedAt = s.Now()
	}
	if err := s.Store.PutSource(scope, source); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, source)
}

func (s *Server) registerAgent(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())
	var agent domain.Agent
	if err := decodeJSON(r, &agent); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	agent.OrganisationID, agent.ProjectID, agent.EnvironmentID = scope.OrganisationID, scope.ProjectID, scope.EnvironmentID
	if agent.CreatedAt.IsZero() {
		agent.CreatedAt, agent.UpdatedAt = s.Now(), s.Now()
	}
	if err := s.Store.RegisterAgent(scope, agent); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())
	var request struct {
		AgentID domain.AgentID `json:"agent_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Store.Heartbeat(scope, request.AgentID, s.Now()); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "online", "server_time": s.Now()})
}

func scopeFromRequest(r *http.Request) (domain.TenantScope, error) {
	scope := domain.TenantScope{
		OrganisationID: domain.OrganisationID(r.Header.Get("X-DBVault-Organisation")),
		ProjectID:      domain.ProjectID(r.Header.Get("X-DBVault-Project")),
		EnvironmentID:  domain.EnvironmentID(r.Header.Get("X-DBVault-Environment")),
		ActorID:        r.Header.Get("X-DBVault-Actor"),
	}
	if !scope.Valid() {
		return scope, errors.New("X-DBVault-Organisation is required")
	}
	return scope, nil
}

type scopeKey struct{}

func scopeFromContext(ctx context.Context) domain.TenantScope {
	scope, _ := ctx.Value(scopeKey{}).(domain.TenantScope)
	return scope
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(discardResponseWriter{}, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status})
}

type discardResponseWriter struct{}

func (discardResponseWriter) Header() http.Header         { return make(http.Header) }
func (discardResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardResponseWriter) WriteHeader(int)             {}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

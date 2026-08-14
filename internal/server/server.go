package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/application/productexperience"
	"github.com/dbvault/dbvault/internal/domain"
)

//go:embed web/dist/*
var embeddedWeb embed.FS

type TLSMode string

const (
	TLSModeACME       TLSMode = "acme"
	TLSModeACMEDNS    TLSMode = "acme_dns"
	TLSModeFiles      TLSMode = "files"
	TLSModeSelfSigned TLSMode = "self_signed"
	TLSModeDisabled   TLSMode = "disabled"
)

type TLSConfig struct {
	Mode        TLSMode
	Email       string
	Certificate string
	PrivateKey  string
}

type Config struct {
	Listen         string
	PublicURL      string
	DataDirectory  string
	SetupTokenPath string
	Version        string
	TLS            TLSConfig
}

type Appliance struct {
	cfg       Config
	api       http.Handler
	assets    fs.FS
	logger    *slog.Logger
	setup     *SetupManager
	px        *productexperience.Service
	readyMu   sync.RWMutex
	ready     bool
	startedAt time.Time
}

func New(cfg Config, api http.Handler, logger *slog.Logger) (*Appliance, error) {
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8080"
	}
	if cfg.DataDirectory == "" {
		cfg.DataDirectory = "/var/lib/dbvault"
	}
	if cfg.SetupTokenPath == "" {
		cfg.SetupTokenPath = filepath.Join(cfg.DataDirectory, "setup-token")
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if logger == nil {
		logger = slog.Default()
	}
	assets, err := fs.Sub(embeddedWeb, "web/dist")
	if err != nil {
		return nil, err
	}
	px, err := productexperience.New(filepath.Join(cfg.DataDirectory, "product-experience"))
	if err != nil {
		return nil, err
	}
	return &Appliance{cfg: cfg, api: api, assets: assets, logger: logger, setup: &SetupManager{Path: cfg.SetupTokenPath}, px: px, startedAt: time.Now().UTC()}, nil
}

func (a *Appliance) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/ready", a.readyHandler)
	mux.HandleFunc("/api/v1/status", a.status)
	mux.HandleFunc("/api/v1/events/jobs", a.jobEvents)
	mux.HandleFunc("/api/v1/jobs", a.jobsRoot)
	mux.HandleFunc("/api/v1/jobs/", a.jobByID)
	mux.HandleFunc("/api/v1/overview", a.overview)
	mux.HandleFunc("/api/v1/inventory", a.inventory)
	mux.HandleFunc("/api/v1/protection-summary", a.protectionSummary)
	mux.HandleFunc("/api/v1/setup", a.setupAPI)
	mux.HandleFunc("/api/v1/setup/finish", a.setupFinish)
	mux.HandleFunc("/api/v1/setup/current-step", a.setupCurrentStep)
	mux.HandleFunc("/api/v1/setup/steps/", a.setupStep)
	mux.HandleFunc("/api/v1/agents/", a.agentProductRoutes)
	mux.HandleFunc("/api/v1/discoveries", a.discoveries)
	mux.HandleFunc("/api/v1/discoveries/", a.discoveryByID)
	mux.HandleFunc("/api/v1/doctor/run", a.doctorRun)
	mux.HandleFunc("/api/v1/policies/simulate", a.policySimulate)
	mux.HandleFunc("/api/v1/sources/", a.sourceProductRoutes)
	mux.HandleFunc("/api/v1/sandboxes", a.sandboxes)
	mux.HandleFunc("/api/v1/sandboxes/", a.sandboxByID)
	mux.HandleFunc("/api/v1/alerts", a.alerts)
	mux.HandleFunc("/api/v1/alerts/", a.alertByID)
	mux.HandleFunc("/api/v1/approvals", a.approvals)
	mux.HandleFunc("/api/v1/recovery-bundles", a.recoveryBundle)
	mux.HandleFunc("/api/v1/support-bundles", a.supportBundle)
	mux.Handle("/api/", http.StripPrefix("/api", optionalHandler(a.api)))
	mux.HandleFunc("/events/jobs", a.jobEvents)
	mux.HandleFunc("/agent/v1/heartbeat", a.agentHeartbeat)
	mux.HandleFunc("/agent/v1/enrol", a.agentEnrol)
	mux.HandleFunc("/install/agent.sh", a.agentInstallScript)
	mux.HandleFunc("/install/checksums.txt", a.checksums)
	mux.HandleFunc("/install/releases/current/manifest.json", a.releaseManifest)
	mux.HandleFunc("/setup/status", a.setupStatus)
	mux.HandleFunc("/setup/complete", a.setupComplete)
	mux.HandleFunc("/favicon.ico", a.favicon)
	mux.HandleFunc("/assets/", a.staticAsset)
	mux.HandleFunc("/", a.spa)
	return securityHeaders(limitHeaders(mux))
}

func (a *Appliance) HTTPServer() *http.Server {
	return &http.Server{Addr: a.cfg.Listen, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
}

func (a *Appliance) SetReady(ready bool) {
	a.readyMu.Lock()
	defer a.readyMu.Unlock()
	a.ready = ready
}

func (a *Appliance) IsReady() bool {
	a.readyMu.RLock()
	defer a.readyMu.RUnlock()
	return a.ready
}

func (a *Appliance) EnsureSetupToken() (string, error) {
	return a.setup.Ensure()
}

func (a *Appliance) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "healthy", "version": a.cfg.Version})
}

func (a *Appliance) readyHandler(w http.ResponseWriter, _ *http.Request) {
	if !a.IsReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (a *Appliance) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ready": a.IsReady(), "version": a.cfg.Version, "uptime_seconds": int(time.Since(a.startedAt).Seconds()), "setup_required": a.setup.Required()})
}

func (a *Appliance) jobEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	writeSSE(w, "hello", "", map[string]any{"status": "connected", "generated_at": time.Now().UTC()})
	last := r.Header.Get("Last-Event-ID")
	if raw := r.URL.Query().Get("last_event_id"); raw != "" {
		last = raw
	}
	for _, event := range a.px.JobEventsSince(last, 100) {
		writeSSE(w, event.EventType, event.EventID, event)
		last = event.EventID
	}
	if flusher != nil {
		flusher.Flush()
	}
	if r.URL.Query().Get("once") == "1" {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			for _, event := range a.px.GenerateDemoProgress() {
				writeSSE(w, event.EventType, event.EventID, event)
				last = event.EventID
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, eventType, eventID string, value any) {
	if eventType != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", eventType)
	}
	if eventID != "" {
		_, _ = fmt.Fprintf(w, "id: %s\n", eventID)
	}
	b, err := json.Marshal(value)
	if err != nil {
		b = []byte(`{"error":"sse marshal failed"}`)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
}

func (a *Appliance) jobsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		filters := map[string]string{
			"status":    r.URL.Query().Get("status"),
			"job_type":  r.URL.Query().Get("job_type"),
			"source_id": r.URL.Query().Get("source_id"),
		}
		writeJSON(w, http.StatusOK, domain.JobListResponse{Jobs: a.px.Jobs(filters), GeneratedAt: time.Now().UTC()})
	case http.MethodPost:
		var request struct {
			JobType      string `json:"job_type"`
			ResourceID   string `json:"resource_id"`
			ResourceName string `json:"resource_name"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request)
		job := a.px.CreateJob(request.JobType, request.ResourceID, request.ResourceName)
		writeJSON(w, http.StatusAccepted, job)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) jobByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		job, ok := a.px.Job(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		job, err := a.px.CancelJob(id)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	if len(parts) == 2 && parts[1] == "retry" && r.Method == http.MethodPost {
		result, err := a.px.RetryJob(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, result)
		return
	}
	if len(parts) == 2 && parts[1] == "logs" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"logs": a.px.JobLogs(id)})
		return
	}
	http.NotFound(w, r)
}

func (a *Appliance) agentHeartbeat(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

func (a *Appliance) agentEnrol(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "enrolment-scaffold", "protocol": 1})
}

func (a *Appliance) agentInstallScript(w http.ResponseWriter, r *http.Request) {
	controller := a.cfg.PublicURL
	if controller == "" {
		controller = schemeFromRequest(r) + "://" + r.Host
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(AgentInstallScript(controller)))
}

func (a *Appliance) checksums(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("# generated during release builds\ndbvault  SHA256_PLACEHOLDER\n"))
}

func (a *Appliance) releaseManifest(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"format": "dbvault-release-manifest", "version": a.cfg.Version, "artifacts": []any{}})
}

func (a *Appliance) setupStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"required": a.setup.Required()})
}

func (a *Appliance) setupComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Token string `json:"token"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := a.setup.Complete(request.Token); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "administrator-created", "email": request.Email})
}

func (a *Appliance) staticAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/assets/")
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "\\") {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, a.assets, name)
}

func (a *Appliance) favicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#0f172a"><rect width="24" height="24" rx="6"/><path d="M12 4 5 7.5v5.3c0 4.6 3.1 8.8 7.5 9.7 4.4-.9 7.5-5.1 7.5-9.7V7.5z" fill="#ffffff"/></svg>`))
}

func (a *Appliance) spa(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if strings.Contains(r.URL.Path, ".") {
			http.NotFound(w, r)
			return
		}
	}
	http.ServeFileFS(w, r, a.assets, "index.html")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func limitHeaders(next http.Handler) http.Handler { return next }

func optionalHandler(h http.Handler) http.Handler {
	if h != nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "api-not-configured"})
	})
}

func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	return "http"
}

func (a *Appliance) inventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.px.Inventory())
}

func (a *Appliance) overview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	value, err := a.px.Overview(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (a *Appliance) protectionSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sourceID := r.URL.Query().Get("source_id")
	if sourceID == "" {
		sourceID = "production-postgres"
	}
	writeJSON(w, http.StatusOK, a.px.ProtectionSummary(r.Context(), domain.SourceID(sourceID)))
}

func (a *Appliance) setupAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.px.Setup())
}

func (a *Appliance) setupStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	step := strings.TrimPrefix(r.URL.Path, "/api/v1/setup/steps/")
	var raw json.RawMessage
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&raw)
	state, err := a.px.SaveSetupStep(step, raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *Appliance) setupCurrentStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Step string `json:"step"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	state, err := a.px.SetSetupStep(request.Step)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *Appliance) setupFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	state, err := a.px.FinishSetup()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *Appliance) agentProductRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "discover" && r.Method == http.MethodPost {
		var request struct {
			Roots []string `json:"roots"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request)
		if len(request.Roots) == 0 {
			request.Roots = []string{"/var/www", "/srv", "/opt"}
		}
		out, err := a.px.Discover(r.Context(), parts[0], request.Roots)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"discoveries": out})
		return
	}
	http.NotFound(w, r)
}

func (a *Appliance) discoveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"discoveries": a.px.Discoveries()})
}

func (a *Appliance) discoveryByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/discoveries/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	status := ""
	switch parts[1] {
	case "adopt":
		status = "adopted"
	case "ignore":
		status = "ignored"
	default:
		http.NotFound(w, r)
		return
	}
	item, err := a.px.UpdateDiscoveryStatus(parts[0], status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *Appliance) doctorRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.px.Doctor(r.Context()))
}

func (a *Appliance) policySimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request domain.PolicySimulationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, a.px.Simulate(request))
}

func (a *Appliance) sourceProductRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sources/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "recovery-timeline" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, a.px.RecoveryTimeline(r.Context(), parts[0]))
		return
	}
	http.NotFound(w, r)
}

func (a *Appliance) sandboxes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		SourceID   string `json:"source_id"`
		SnapshotID string `json:"snapshot_id"`
		Engine     string `json:"engine"`
		CreatedBy  string `json:"created_by"`
		TargetTime string `json:"target_time"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var target *time.Time
	if request.TargetTime != "" {
		parsed, err := time.Parse(time.RFC3339, request.TargetTime)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_time must be RFC3339"})
			return
		}
		target = &parsed
	}
	sandbox := a.px.CreateSandbox(request.SourceID, request.SnapshotID, request.Engine, request.CreatedBy, target)
	job := a.px.CreateJob("sandbox_restore", sandbox.ID, "Sandbox restore")
	sandbox.JobID = job.ID
	writeJSON(w, http.StatusAccepted, sandbox)
}

func (a *Appliance) sandboxByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/sandboxes/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		sb, ok := a.px.GetSandbox(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, sb)
	case http.MethodDelete:
		if !a.px.DeleteSandbox(id) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) alerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed < 500 {
			limit = parsed
		}
	}
	alerts := a.px.Alerts()
	if len(alerts) > limit {
		alerts = alerts[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": alerts})
}

func (a *Appliance) alertByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/alerts/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	status := ""
	switch parts[1] {
	case "acknowledge":
		status = "acknowledged"
	case "resolve":
		status = "resolved"
	default:
		http.NotFound(w, r)
		return
	}
	alert, err := a.px.UpdateAlertStatus(parts[0], status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, alert)
}

func (a *Appliance) approvals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"approvals": a.px.RestoreApprovals()})
	case http.MethodPost:
		var request struct {
			SourceID    string `json:"source_id"`
			Reason      string `json:"reason"`
			TargetTime  string `json:"target_time"`
			Target      string `json:"target"`
			RequestedBy string `json:"requested_by"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		var targetTime *time.Time
		if request.TargetTime != "" {
			parsed, err := time.Parse(time.RFC3339, request.TargetTime)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target_time must be RFC3339"})
				return
			}
			targetTime = &parsed
		}
		approval, err := a.px.CreateRestoreApproval(request.SourceID, request.Reason, targetTime, request.Target, request.RequestedBy)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, approval)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) recoveryBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path, err := a.px.CreateRecoveryBundle(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"path": path, "status": "created"})
}

func (a *Appliance) supportBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path, err := a.px.CreateSupportBundle(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"path": path, "status": "created"})
}

func AgentInstallScript(controller string) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
CONTROLLER=%q
TOKEN="${DBVAULT_AGENT_TOKEN:-${1:-}}"
if [ -z "$TOKEN" ]; then
  echo "missing enrolment token: set DBVAULT_AGENT_TOKEN or pass token as first argument" >&2
  exit 2
fi
if command -v dbvault >/dev/null 2>&1; then
  exec dbvault agent install --controller "$CONTROLLER" --token "$TOKEN"
fi
echo "dbvault binary not found. Download the signed release first, then run dbvault agent install." >&2
exit 1
`, controller)
}

// SetupManager owns the one-time first-run setup token.
type SetupManager struct{ Path string }

func (s *SetupManager) Ensure() (string, error) {
	if s.Path == "" {
		return "", errors.New("setup token path is empty")
	}
	if b, err := os.ReadFile(s.Path); err == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b)), nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return "", err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := strings.ToUpper(hex.EncodeToString(buf[:4])) + "-" + strings.ToUpper(hex.EncodeToString(buf[4:8])) + "-" + strings.ToUpper(hex.EncodeToString(buf[8:12])) + "-" + strings.ToUpper(hex.EncodeToString(buf[12:16])) + "-" + strings.ToUpper(hex.EncodeToString(buf[16:20])) + "-" + strings.ToUpper(hex.EncodeToString(buf[20:24]))
	if err := os.WriteFile(s.Path, []byte(token+"\n"), 0600); err != nil {
		return "", err
	}
	return token, nil
}

func (s *SetupManager) Required() bool {
	if s.Path == "" {
		return true
	}
	b, err := os.ReadFile(s.Path)
	return err == nil && strings.TrimSpace(string(b)) != ""
}

func (s *SetupManager) Complete(token string) error {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return err
	}
	expected := strings.TrimSpace(string(b))
	provided := strings.TrimSpace(token)
	if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return errors.New("invalid setup token")
	}
	return os.Remove(s.Path)
}

func Run(ctx context.Context, appliance *Appliance) error {
	server := appliance.HTTPServer()
	errc := make(chan error, 1)
	go func() { errc <- server.ListenAndServe() }()
	appliance.SetReady(true)
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

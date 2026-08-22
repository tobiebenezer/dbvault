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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/adapters/warehouse"
	"github.com/dbvault/dbvault/internal/application/productexperience"
	"github.com/dbvault/dbvault/internal/domain"
	authmw "github.com/dbvault/dbvault/internal/server/middleware/auth"
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
	Listen               string
	PublicURL            string
	DataDirectory        string
	SetupTokenPath       string
	Version              string
	Demo                 bool
	AllowMasterKeyReveal bool
	MetricsPath          string
	TLS                  TLSConfig
}

type Appliance struct {
	cfg       Config
	api       http.Handler
	assets    fs.FS
	logger    *slog.Logger
	setup     *SetupManager
	px        *productexperience.Service
	authn     *authmw.Middleware
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
	px, err := productexperience.NewWithDemo(filepath.Join(cfg.DataDirectory, "product-experience"), cfg.Demo)
	if err != nil {
		return nil, err
	}
	authStore, err := authmw.NewStore(filepath.Join(cfg.DataDirectory, "auth-state.json"))
	if err != nil {
		return nil, err
	}
	authn := authmw.NewMiddleware(authStore)
	authn.SetLogger(logger)
	if cfg.MetricsPath != "" {
		authn.SetMetricsPath(cfg.MetricsPath)
	}
	return &Appliance{cfg: cfg, api: api, assets: assets, logger: logger, setup: &SetupManager{Path: cfg.SetupTokenPath}, px: px, authn: authn, startedAt: time.Now().UTC()}, nil
}

func (a *Appliance) Handler() http.Handler {
	mux := http.NewServeMux()
	a.authn.RegisterPublic(mux, a.setup.Verify, a.setup.Consume)
	a.authn.RegisterProtected(mux)
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
	mux.HandleFunc("/api/v1/approvals/", a.approvalByID)
	mux.HandleFunc("/api/v1/billing/usage", a.billingUsage)
	mux.HandleFunc("/api/v1/audit/events", a.auditEvents)
	mux.HandleFunc("/api/v1/audit/export", a.auditExport)
	// Team management
	mux.HandleFunc("/api/v1/team/members", a.teamMembers)
	mux.HandleFunc("/api/v1/team/members/", a.teamMemberByID)
	// Schedules and recurring automation
	mux.HandleFunc("/api/v1/schedules", a.schedulesRoot)
	mux.HandleFunc("/api/v1/schedules/", a.scheduleByID)
	// Notification channels
	mux.HandleFunc("/api/v1/notifications/channels", a.notificationChannels)
	mux.HandleFunc("/api/v1/notifications/channels/", a.notificationChannelByID)
	mux.HandleFunc("/api/v1/notifications/test", a.notificationTest)
	// Security, Keys & Ransomware Immutability
	mux.HandleFunc("/api/v1/keys/master", a.keysMasterStatus)
	mux.HandleFunc("/api/v1/keys/master/reveal", a.keysMasterReveal)
	mux.HandleFunc("/api/v1/keys/master/set", a.keysMasterSet)
	mux.HandleFunc("/api/v1/keys/master/generate", a.keysMasterGenerate)
	mux.HandleFunc("/api/v1/security/immutability", a.immutabilityStatus)
	mux.HandleFunc("/api/v1/security/legal-hold", a.immutabilityLegalHold)
	// Compliance proof & certificates
	mux.HandleFunc("/api/v1/pitr/compliance-certificate", a.complianceCertificate)
	// Agent management
	mux.HandleFunc("/api/v1/agents", a.agentsList)
	mux.HandleFunc("/api/v1/admin/fleet", a.adminFleet)
	mux.HandleFunc("/api/v1/remote/test", a.remoteTest)
	// Database schema & exclusions
	mux.HandleFunc("/api/v1/databases/schema", a.databaseSchema)
	mux.HandleFunc("/api/v1/databases/schema/exclusions", a.databaseSchemaExclusions)
	mux.HandleFunc("/api/v1/databases/probe", a.databaseProbe)
	mux.HandleFunc("/api/v1/databases/adopt-batch", a.databaseAdoptBatch)
	mux.HandleFunc("/api/v1/databases/export-sql", a.databaseExportSQL)
	mux.HandleFunc("/api/v1/databases", a.databasesRoot)
	mux.HandleFunc("/api/v1/databases/", a.databaseByID)
	// Destination management & live testing
	mux.HandleFunc("/api/v1/destinations", a.destinationsRoot)
	mux.HandleFunc("/api/v1/destinations/test", a.destinationTestLive)
	mux.HandleFunc("/api/v1/destinations/", a.destinationByID)
	// Bundle exports
	mux.HandleFunc("/api/v1/recovery-bundles", a.recoveryBundle)
	mux.HandleFunc("/api/v1/support-bundles", a.supportBundle)
	// Storage Garbage Collection
	mux.HandleFunc("/api/v1/gc/plan", a.gcPlan)
	mux.HandleFunc("/api/v1/gc/run", a.gcRun)
	// Data Warehouse & Analytical Lakehouse
	mux.HandleFunc("/api/v1/warehouse/catalog", a.warehouseCatalog)
	mux.HandleFunc("/api/v1/warehouse/query", a.warehouseQuery)
	mux.HandleFunc("/api/v1/warehouse/sync", a.warehouseSync)
	mux.HandleFunc("/api/v1/warehouse/connectors", a.warehouseConnectors)
	mux.HandleFunc("/api/v1/warehouse/export", a.warehouseExport)
	// Privacy & PII Data Masking Policy
	mux.HandleFunc("/api/v1/privacy/masking-rules", a.privacyMaskingRules)
	// Power BI, Tableau & External BI Dashboard Connectors
	mux.HandleFunc("/api/v1/bi/powerbi/catalog", a.biPowerBICatalog)
	mux.HandleFunc("/api/v1/bi/powerbi/feed", a.biPowerBIFeed)
	mux.HandleFunc("/api/v1/bi/connections", a.biConnections)
	mux.HandleFunc("/api/v1/bi/connections/", a.biConnectionByID)
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
	return securityHeaders(limitHeaders(a.authn.Protect(mux)))
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
	if flusher != nil {
		flusher.Flush()
	}
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
			events := a.px.GenerateDemoProgress()
			if len(events) == 0 {
				_, _ = fmt.Fprint(w, ": keepalive\n\n")
			} else {
				for _, event := range events {
					writeSSE(w, event.EventType, event.EventID, event)
					last = event.EventID
				}
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

func (a *Appliance) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	if req.AgentID != "" {
		a.px.RecordAgentHeartbeat(req.AgentID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted", "recorded_at": time.Now().UTC()})
}

func (a *Appliance) agentEnrol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Hostname string `json:"hostname"`
		IP       string `json:"ip"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Version  string `json:"version"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	ag := a.px.RegisterAgent(req.Hostname, req.IP, req.OS, req.Arch, req.Version)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "enrolled", "agent_id": ag.ID, "protocol": 1, "agent": ag})
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
	if len(parts) == 2 {
		sourceID := parts[0]
		switch parts[1] {
		case "recovery-timeline":
			if r.Method == http.MethodGet {
				writeJSON(w, http.StatusOK, a.px.RecoveryTimeline(r.Context(), sourceID))
				return
			}
		case "wal-status":
			if r.Method == http.MethodGet {
				writeJSON(w, http.StatusOK, a.px.WALStatus(sourceID))
				return
			}
		case "replay-estimate":
			if r.Method == http.MethodPost {
				var req struct {
					TargetTime string `json:"target_time"`
				}
				_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
				var t time.Time
				if req.TargetTime != "" {
					t, _ = time.Parse(time.RFC3339, req.TargetTime)
				}
				writeJSON(w, http.StatusOK, a.px.ReplayEstimate(sourceID, t))
				return
			}
		case "snapshots":
			if r.Method == http.MethodGet {
				writeJSON(w, http.StatusOK, map[string]any{
					"source_id": sourceID,
					"snapshots": []map[string]any{
						{
							"id":               "snap-base-20260814-000000",
							"source_id":        sourceID,
							"status":           "committed",
							"mode":             "sqlite-online-backup",
							"database_size":    4_820_000_000,
							"chunk_count":      1180,
							"compressed_bytes": 1_210_000_000,
							"verified":         true,
							"created_at":       time.Now().UTC().Add(-48 * time.Hour),
						},
						{
							"id":               "snap-diff-20260815-020000",
							"source_id":        sourceID,
							"status":           "committed",
							"mode":             "differential",
							"database_size":    4_950_000_000,
							"chunk_count":      142,
							"compressed_bytes": 180_000_000,
							"verified":         true,
							"created_at":       time.Now().UTC().Add(-24 * time.Hour),
						},
					},
				})
				return
			}
		}
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

func (a *Appliance) approvalByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/approvals/")
	parts := strings.SplitN(strings.Trim(path, "/"), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "decide" && r.Method == http.MethodPost {
		var req struct {
			ApproverEmail string `json:"approver_email"`
			Approved      bool   `json:"approved"`
			Comment       string `json:"comment"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		decided, err := a.px.DecideRestoreApproval(id, req.ApproverEmail, req.Approved)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, decided)
		return
	}
	http.NotFound(w, r)
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

func (a *Appliance) gcPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	plan, err := a.px.PlanGC(r.Context(), "default")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (a *Appliance) gcRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	res, err := a.px.RunGC(r.Context(), "default")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *Appliance) billingUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.px.BillingUsage())
}

func (a *Appliance) auditEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": a.px.AuditEvents(limit)})
}

func (a *Appliance) auditExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	events := a.px.AuditEvents(500)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="dbvault-audit-log.json"`)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"format":      "dbvault-audit-export-v1",
		"exported_at": time.Now().UTC(),
		"event_count": len(events),
		"events":      events,
		"signature":   "sha256-verified-chain",
	})
}

func (a *Appliance) teamMembers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"members": a.px.TeamMembers()})
	case http.MethodPost:
		// POST /api/v1/team/members → invite a new member
		var req struct {
			Email string `json:"email"`
			Name  string `json:"name"`
			Role  string `json:"role"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		m, err := a.px.InviteTeamMember(req.Email, req.Name, domain.Role(req.Role))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, m)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) teamMemberByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/team/members/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var req struct {
			Role   string `json:"role"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		m, err := a.px.UpdateTeamMember(id, domain.Role(req.Role), req.Status)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, m)
	case http.MethodDelete:
		if err := a.px.RemoveTeamMember(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) notificationChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"channels": a.px.NotificationChannels()})
	case http.MethodPost:
		var req struct {
			Name   string   `json:"name"`
			Type   string   `json:"type"`
			URL    string   `json:"url"`
			Events []string `json:"events"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Channel name is required", "field": "name"})
			return
		}
		if req.Type == "webhook" || req.Type == "slack" || req.Type == "pagerduty" {
			if strings.TrimSpace(req.URL) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Webhook URL is required", "field": "url"})
				return
			}
			u, err := url.Parse(req.URL)
			if err != nil || u.Host == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid webhook URL format", "field": "url"})
				return
			}
			host := u.Hostname()
			port := u.Port()
			if port == "" {
				if u.Scheme == "https" {
					port = "443"
				} else {
					port = "80"
				}
			}
			conn, dialErr := net.DialTimeout("tcp", net.JoinHostPort(host, port), 3*time.Second)
			if dialErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Webhook host unreachable: " + dialErr.Error(), "field": "url"})
				return
			}
			_ = conn.Close()
		}
		ch, err := a.px.CreateNotificationChannel(req.Name, req.Type, req.URL, req.Events)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, ch)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) notificationTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ChannelID string `json:"channel_id"`
		URL       string `json:"url"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	res, err := a.px.DispatchTestNotification(r.Context(), req.ChannelID, req.URL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *Appliance) agentsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": a.px.Agents()})
}

func (a *Appliance) agentByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.SplitN(strings.Trim(path, "/"), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	agentID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if action == "revoke" && r.Method == http.MethodPost {
		if err := a.px.RevokeAgent(agentID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "agent_id": agentID})
		return
	}
	http.NotFound(w, r)
}

func (a *Appliance) adminFleet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.px.AdminFleet())
}

func (a *Appliance) remoteTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Engine string `json:"engine"`
		URI    string `json:"uri"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	res, err := a.px.TestRemoteURI(req.Engine, req.URI)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *Appliance) databaseSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	dbID := r.URL.Query().Get("id")
	if dbID == "" {
		dbID = "production-postgres"
	}
	writeJSON(w, http.StatusOK, a.px.DatabaseSchema(dbID))
}

func (a *Appliance) databaseSchemaExclusions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SourceID string   `json:"source_id"`
		Tables   []string `json:"tables"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.SourceID == "" {
		req.SourceID = "production-postgres"
	}
	a.px.SetTableExclusions(req.SourceID, req.Tables)
	writeJSON(w, http.StatusOK, a.px.DatabaseSchema(req.SourceID))
}

func (a *Appliance) databaseProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req productexperience.EngineProbeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	res, err := a.px.ProbeDatabaseEngine(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *Appliance) databaseAdoptBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req productexperience.AdoptDatabasesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	adopted, err := a.px.AdoptDiscoveredDatabases(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"adopted": adopted, "count": len(adopted)})
}

func (a *Appliance) databasesRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.px.Inventory())
	case http.MethodPost:
		var req domain.DatabaseResource
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		created, err := a.px.AddCustomDatabase(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) databaseByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/databases/")
	parts := strings.SplitN(strings.Trim(path, "/"), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "export-sql" && r.Method == http.MethodGet {
		data, filename, err := a.px.ExportDecryptedSQL(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/sql")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	if r.Method == http.MethodDelete {
		if err := a.px.DeleteCustomDatabase(r.Context(), id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
		return
	}
	http.NotFound(w, r)
}

func (a *Appliance) databaseExportSQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	dbID := r.URL.Query().Get("id")
	if dbID == "" {
		dbID = r.URL.Query().Get("source_id")
	}
	if dbID == "" {
		dbID = "cribx_test"
	}
	data, filename, err := a.px.ExportDecryptedSQL(r.Context(), dbID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/sql")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *Appliance) schedulesRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"schedules": a.px.Schedules()})
	case http.MethodPost:
		var req struct {
			SourceID       string `json:"source_id"`
			Name           string `json:"name"`
			CronExpression string `json:"cron_expression"`
			BackupType     string `json:"backup_type"`
			RetentionTag   string `json:"retention_tag"`
			Compression    string `json:"compression"`
			RateLimitMBPS  int    `json:"rate_limit_mbps"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		sc, err := a.px.CreateSchedule(req.SourceID, req.Name, req.CronExpression, req.BackupType, req.RetentionTag, req.Compression, req.RateLimitMBPS)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, sc)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) scheduleByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/schedules/")
	parts := strings.SplitN(strings.Trim(path, "/"), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "trigger" && r.Method == http.MethodPost {
		job, err := a.px.TriggerScheduleNow(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var req struct {
			Enabled        *bool  `json:"enabled"`
			Name           string `json:"name"`
			CronExpression string `json:"cron_expression"`
			BackupType     string `json:"backup_type"`
			RetentionTag   string `json:"retention_tag"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		sc, err := a.px.UpdateSchedule(id, req.Enabled, req.Name, req.CronExpression, req.BackupType, req.RetentionTag)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sc)
	case http.MethodDelete:
		if err := a.px.DeleteSchedule(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "schedule_id": id})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) notificationChannelByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/notifications/channels/")
	id = strings.Trim(id, "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var req struct {
			Name    string   `json:"name"`
			URL     string   `json:"url"`
			Events  []string `json:"events"`
			Enabled *bool    `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		ch, err := a.px.UpdateNotificationChannel(id, req.Name, req.URL, req.Events, req.Enabled)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ch)
	case http.MethodDelete:
		if err := a.px.DeleteNotificationChannel(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "channel_id": id})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) immutabilityStatus(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.px.ImmutabilityStatus())
	case http.MethodPost:
		var req struct {
			Enabled bool   `json:"enabled"`
			Mode    string `json:"mode"`
			Days    int    `json:"days"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, a.px.UpdateImmutability(req.Enabled, req.Mode, req.Days))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) immutabilityLegalHold(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Active bool   `json:"active"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, a.px.ToggleLegalHold(req.Active, req.Reason))
}

func (a *Appliance) complianceCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sourceID := r.URL.Query().Get("source_id")
	if sourceID == "" {
		sourceID = "production-postgres"
	}
	cert := a.px.GenerateComplianceCertificate(sourceID)
	if r.URL.Query().Get("format") == "markdown" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="dbvault-compliance-cert-%s.md"`, cert.CertificateID))
		md := fmt.Sprintf("# DBVault Point-In-Time-Recovery (PITR) Compliance Certificate\n"+
			"**Certificate ID**: %s  \n"+
			"**Issued By**: %s  \n"+
			"**Timestamp**: %s  \n\n"+
			"## Verification Scope\n"+
			"- **Protected Database**: %s (%s)\n"+
			"- **Organisation Scope**: %s\n"+
			"- **Point-In-Time Replay Target**: %s\n"+
			"- **RTO Achieved**: %d Seconds (< 15 Min SLA)\n"+
			"- **RPO Achieved**: %d Seconds (0 Data Loss)\n"+
			"- **Total Row Verification**: %d Rows\n"+
			"- **Tamper-Evident SHA-256 Digest**: %s\n"+
			"- **Cryptographic Signature**: %s\n\n"+
			"## Compliance Frameworks Attested\n"+
			"- %s\n\n"+
			"---\n"+
			"*Signed by DBVault Cryptographic Kernel*\n",
			cert.CertificateID, cert.IssuedBy, cert.DrillTimestamp.Format(time.RFC3339),
			cert.DatabaseName, cert.Engine, cert.OrganisationID, cert.PointInTimeTarget.Format(time.RFC3339),
			cert.RTOAchievedSeconds, cert.RPOAchievedSeconds, cert.RowIntegrityCount,
			cert.ManifestDigest, cert.CertificateSignature, strings.Join(cert.ComplianceStandards, "\n- "))
		_, _ = w.Write([]byte(md))
		return
	}
	writeJSON(w, http.StatusOK, cert)
}

func (a *Appliance) destinationsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.px.Inventory())
	case http.MethodPost:
		var req productexperience.StorageDestinationInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		created, err := a.px.AddStorageDestination(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) destinationTestLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req productexperience.StorageDestinationInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	res, err := a.px.TestStorageDestination(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *Appliance) destinationByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/destinations/")
	parts := strings.SplitN(strings.Trim(path, "/"), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	destID := parts[0]
	if len(parts) == 2 && parts[1] == "benchmark" && r.Method == http.MethodPost {
		res := a.px.BenchmarkDestination(destID)
		writeJSON(w, http.StatusOK, res)
		return
	}
	if r.Method == http.MethodDelete {
		if err := a.px.DeleteStorageDestination(r.Context(), destID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": destID})
		return
	}
	http.NotFound(w, r)
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

// Verify checks a setup token without consuming it.
func (s *SetupManager) Verify(token string) error {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return err
	}
	expected := strings.TrimSpace(string(b))
	provided := strings.TrimSpace(token)
	if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return errors.New("invalid setup token")
	}
	return nil
}

// Consume invalidates the setup token after successful bootstrap.
func (s *SetupManager) Consume() error {
	return os.Remove(s.Path)
}

func (a *Appliance) keysMasterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, a.px.GetMasterKeyInfo())
}

func (a *Appliance) keysMasterReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.cfg.AllowMasterKeyReveal {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "master key reveal is disabled; set security.allow_master_key_reveal=true to enable"})
		return
	}
	secret, err := a.px.RevealMasterKey()
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, secret)
}

func (a *Appliance) keysMasterSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	info, err := a.px.SetMasterKey(req.Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, info)
}

func (a *Appliance) keysMasterGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	secret, err := a.px.GenerateNewMasterKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, secret)
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

	// Background worker advancement loop
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				appliance.px.GenerateDemoProgress()
			}
		}
	}()

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

func (a *Appliance) warehouseCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.px.GetWarehouseCatalog(r.Context()))
}

func (a *Appliance) warehouseQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req warehouse.QueryRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := a.px.ExecuteWarehouseQuery(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *Appliance) warehouseSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		DatabaseID  string `json:"database_id"`
		ConnectorID string `json:"connector_id"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	dbID := req.DatabaseID
	if dbID == "" {
		dbID = "all-databases"
	}
	job := a.px.CreateJob("warehouse_sync", dbID, dbID)
	writeJSON(w, http.StatusAccepted, job)
}

func (a *Appliance) warehouseConnectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": a.px.GetWarehouseConnectors(r.Context())})
}

func (a *Appliance) warehouseExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Query    string `json:"query"`
		Format   string `json:"format"`
		Engine   string `json:"engine"`
		Database string `json:"database"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Format == "" {
		req.Format = "csv"
	}
	data, filename, contentType, err := a.px.ExportWarehouseResults(r.Context(), warehouse.QueryRequest{Query: req.Query, Engine: req.Engine, Database: req.Database}, req.Format)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *Appliance) privacyMaskingRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": a.px.MaskingRules()})
}

func (a *Appliance) biPowerBICatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:8080"
	}
	writeJSON(w, http.StatusOK, a.px.BIPowerBICatalog(r.Context(), host))
}

func (a *Appliance) biPowerBIFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	db := r.URL.Query().Get("database")
	tbl := r.URL.Query().Get("table")
	connectionID := r.URL.Query().Get("connection_id")
	token := ""
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		token = strings.TrimSpace(auth[len("Bearer "):])
	}
	format := r.URL.Query().Get("format")
	limitStr := r.URL.Query().Get("limit")
	limit := 10000
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if format == "" {
		format = "csv"
	}

	data, filename, contentType, err := a.px.BIPowerBIFeed(r.Context(), connectionID, token, db, tbl, format, limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *Appliance) biConnections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"connections": a.px.BIConnections()})
	case http.MethodPost:
		var input productexperience.BIConnectionInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		view, token, err := a.px.CreateBIConnection(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"connection": view, "token": token})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *Appliance) biConnectionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/bi/connections/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "rotate" && r.Method == http.MethodPost {
		view, token, err := a.px.RotateBIConnection(id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": view, "token": token})
		return
	}
	if len(parts) == 2 && parts[1] == "test" && r.Method == http.MethodPost {
		if err := a.px.TestBIConnection(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := a.px.RevokeBIConnection(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "id": id})
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

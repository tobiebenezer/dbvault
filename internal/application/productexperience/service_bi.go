package productexperience

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var biIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var biDatabaseIdentifier = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// BIDatasetScope is the smallest unit a BI connection can read.
type BIDatasetScope struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

// BIConnection stores only a hash of the bearer token. The plaintext token is
// returned exactly once by CreateBIConnection or RotateBIConnection.
type BIConnection struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Provider   string           `json:"provider"`
	Datasets   []BIDatasetScope `json:"datasets"`
	TokenHash  string           `json:"token_hash"`
	CreatedAt  time.Time        `json:"created_at"`
	LastUsedAt *time.Time       `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time       `json:"expires_at,omitempty"`
	RevokedAt  *time.Time       `json:"revoked_at,omitempty"`
}

type BIConnectionView struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Provider   string           `json:"provider"`
	Datasets   []BIDatasetScope `json:"datasets"`
	CreatedAt  time.Time        `json:"created_at"`
	LastUsedAt *time.Time       `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time       `json:"expires_at,omitempty"`
	RevokedAt  *time.Time       `json:"revoked_at,omitempty"`
	Status     string           `json:"status"`
}

type BIConnectionInput struct {
	Name      string           `json:"name"`
	Provider  string           `json:"provider"`
	Datasets  []BIDatasetScope `json:"datasets"`
	ExpiresIn int              `json:"expires_in_days"`
}

func validBIDataset(scope BIDatasetScope) bool {
	return (scope.Database == "" || scope.Database == "duckdb" || scope.Database == "auto" || biDatabaseIdentifier.MatchString(scope.Database)) && biIdentifier.MatchString(scope.Table)
}

func normalizeBIProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func validBIProvider(provider string) bool {
	switch normalizeBIProvider(provider) {
	case "powerbi", "excel", "tableau", "metabase", "superset", "python":
		return true
	default:
		return false
	}
}

func (s *Service) listBIConnectionsLocked() []BIConnectionView {
	out := make([]BIConnectionView, 0, len(s.biConnections))
	for _, c := range s.biConnections {
		status := "active"
		if c.RevokedAt != nil {
			status = "revoked"
		} else if c.ExpiresAt != nil && !c.ExpiresAt.After(s.now()) {
			status = "expired"
		}
		out = append(out, BIConnectionView{ID: c.ID, Name: c.Name, Provider: c.Provider, Datasets: append([]BIDatasetScope(nil), c.Datasets...), CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt, ExpiresAt: c.ExpiresAt, RevokedAt: c.RevokedAt, Status: status})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func newBIBearer() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := "dbv_" + base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}

func (s *Service) CreateBIConnection(input BIConnectionInput) (BIConnectionView, string, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Provider = normalizeBIProvider(input.Provider)
	if input.Name == "" || len(input.Name) > 120 {
		return BIConnectionView{}, "", fmt.Errorf("connection name is required and must be at most 120 characters")
	}
	if !validBIProvider(input.Provider) {
		return BIConnectionView{}, "", fmt.Errorf("unsupported BI provider %q", input.Provider)
	}
	if len(input.Datasets) == 0 || len(input.Datasets) > 100 {
		return BIConnectionView{}, "", fmt.Errorf("select at least one dataset")
	}
	seen := map[string]bool{}
	for _, dataset := range input.Datasets {
		if !validBIDataset(dataset) {
			return BIConnectionView{}, "", fmt.Errorf("invalid dataset scope")
		}
		key := dataset.Database + "." + dataset.Table
		if seen[key] {
			return BIConnectionView{}, "", fmt.Errorf("duplicate dataset scope %q", key)
		}
		seen[key] = true
	}
	token, tokenHash, err := newBIBearer()
	if err != nil {
		return BIConnectionView{}, "", fmt.Errorf("generate BI token: %w", err)
	}
	now := s.now()
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return BIConnectionView{}, "", err
	}
	c := BIConnection{ID: "bic_" + hex.EncodeToString(idBytes), Name: input.Name, Provider: input.Provider, Datasets: append([]BIDatasetScope(nil), input.Datasets...), TokenHash: tokenHash, CreatedAt: now}
	if input.ExpiresIn > 0 {
		if input.ExpiresIn > 3650 {
			return BIConnectionView{}, "", fmt.Errorf("expiration cannot exceed 3650 days")
		}
		expires := now.Add(time.Duration(input.ExpiresIn) * 24 * time.Hour)
		c.ExpiresAt = &expires
	}
	s.mu.Lock()
	if s.biConnections == nil {
		s.biConnections = map[string]BIConnection{}
	}
	s.biConnections[c.ID] = c
	s.savePersistedDataLocked()
	s.mu.Unlock()
	return s.biConnectionView(c), token, nil
}

func (s *Service) biConnectionView(c BIConnection) BIConnectionView {
	status := "active"
	if c.RevokedAt != nil {
		status = "revoked"
	} else if c.ExpiresAt != nil && !c.ExpiresAt.After(s.now()) {
		status = "expired"
	}
	return BIConnectionView{ID: c.ID, Name: c.Name, Provider: c.Provider, Datasets: append([]BIDatasetScope(nil), c.Datasets...), CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt, ExpiresAt: c.ExpiresAt, RevokedAt: c.RevokedAt, Status: status}
}

func (s *Service) BIConnections() []BIConnectionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listBIConnectionsLocked()
}

func (s *Service) RotateBIConnection(id string) (BIConnectionView, string, error) {
	token, hash, err := newBIBearer()
	if err != nil {
		return BIConnectionView{}, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.biConnections[id]
	if !ok {
		return BIConnectionView{}, "", fmt.Errorf("BI connection %q not found", id)
	}
	if c.RevokedAt != nil {
		return BIConnectionView{}, "", fmt.Errorf("BI connection is revoked")
	}
	c.TokenHash = hash
	c.LastUsedAt = nil
	s.biConnections[id] = c
	s.savePersistedDataLocked()
	return s.biConnectionView(c), token, nil
}

func (s *Service) RevokeBIConnection(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.biConnections[id]
	if !ok {
		return fmt.Errorf("BI connection %q not found", id)
	}
	if c.RevokedAt == nil {
		now := s.now()
		c.RevokedAt = &now
		c.TokenHash = ""
		s.biConnections[id] = c
		s.savePersistedDataLocked()
	}
	return nil
}

func (s *Service) AuthorizeBIFeed(connectionID, token, database, table string) error {
	if connectionID == "" || token == "" {
		return fmt.Errorf("BI connection credentials are required")
	}
	hash := sha256.Sum256([]byte(token))
	hashText := hex.EncodeToString(hash[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.biConnections[connectionID]
	if !ok || c.TokenHash == "" || !strings.EqualFold(c.TokenHash, hashText) {
		return fmt.Errorf("invalid BI connection credentials")
	}
	if c.RevokedAt != nil {
		return fmt.Errorf("BI connection has been revoked")
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(s.now()) {
		return fmt.Errorf("BI connection has expired")
	}
	requested := BIDatasetScope{Database: database, Table: table}
	allowed := false
	for _, scope := range c.Datasets {
		if scope.Table == requested.Table && (scope.Database == "" || scope.Database == requested.Database || (scope.Database == "duckdb" && (requested.Database == "" || requested.Database == "duckdb"))) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("dataset is not included in this BI connection")
	}
	now := s.now()
	c.LastUsedAt = &now
	s.biConnections[connectionID] = c
	// Last-used telemetry is best effort; persistence failures must not leak data.
	s.savePersistedDataLocked()
	return nil
}

func (s *Service) TestBIConnection(id string) error {
	s.mu.Lock()
	c, ok := s.biConnections[id]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("BI connection %q not found", id)
	}
	if c.RevokedAt != nil {
		return fmt.Errorf("BI connection has been revoked")
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(s.now()) {
		return fmt.Errorf("BI connection has expired")
	}
	return nil
}

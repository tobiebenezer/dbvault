// Package auth provides the appliance trust boundary: admin credential
// bootstrap through the setup-token flow, session cookies for the web
// console, hashed bearer tokens for API clients, and fail-closed request
// protection with a per-IP login rate limit.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"

	pbkdf2Iterations = 100_000
	keyLen           = 32

	defaultSessionTTL = 24 * time.Hour
	defaultTokenTTL   = 30 * 24 * time.Hour
)

// Principal is the authenticated identity attached to a request.
type Principal struct {
	Subject string
	Role    string
	Via     string // "session" or "bearer"
}

// credential is a PBKDF2-SHA256 password record.
type credential struct {
	Username string `json:"username"`
	Salt     []byte `json:"salt"`
	Hash     []byte `json:"hash"`
}

type session struct {
	id        string
	subject   string
	role      string
	expiresAt time.Time
}

type apiToken struct {
	hash      [sha256.Size]byte
	subject   string
	role      string
	expiresAt time.Time
}

// persistedState is the on-disk form of the store. Only hashes are persisted;
// session identifiers live exclusively in memory.
type persistedState struct {
	Admin  *credential      `json:"admin,omitempty"`
	Tokens []persistedToken `json:"tokens,omitempty"`
}

type persistedToken struct {
	Hash      string    `json:"hash"`
	Subject   string    `json:"subject"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store holds admin credentials, console sessions and API tokens.
type Store struct {
	mu         sync.Mutex
	path       string // persistence path; empty = memory only
	admin      *credential
	sessions   map[string]*session // keyed by raw session ID
	tokens     map[string]*apiToken
	now        func() time.Time
	sessionTTL time.Duration
	tokenTTL   time.Duration
}

// NewStore returns a store persisting to path when path is non-empty.
func NewStore(path string) (*Store, error) {
	s := &Store{
		path:       path,
		sessions:   map[string]*session{},
		tokens:     map[string]*apiToken{},
		now:        func() time.Time { return time.Now().UTC() },
		sessionTTL: defaultSessionTTL,
		tokenTTL:   defaultTokenTTL,
	}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	var state persistedState
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, fmt.Errorf("auth state %s is corrupt: %w", path, err)
	}
	s.admin = state.Admin
	for _, t := range state.Tokens {
		raw, decErr := hex.DecodeString(t.Hash)
		if decErr != nil || len(raw) != sha256.Size {
			continue
		}
		var hash [sha256.Size]byte
		copy(hash[:], raw)
		s.tokens[t.Hash] = &apiToken{hash: hash, subject: t.Subject, role: t.Role, expiresAt: t.ExpiresAt}
	}
	return s, nil
}

func (s *Store) saveLocked() {
	if s.path == "" {
		return
	}
	state := persistedState{Admin: s.admin}
	for h, t := range s.tokens {
		if s.now().Before(t.expiresAt) {
			state.Tokens = append(state.Tokens, persistedToken{Hash: h, Subject: t.subject, Role: t.role, ExpiresAt: t.expiresAt})
		}
	}
	b, err := json.Marshal(state)
	if err != nil {
		return
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0700)
	}
	_ = os.WriteFile(s.path, b, 0600)
}

// Bootstrapped reports whether an administrator credential exists.
func (s *Store) Bootstrapped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.admin != nil
}

// BootstrapAdmin creates the initial administrator credential. It refuses to
// overwrite an existing one so the setup window can never be re-opened.
func (s *Store) BootstrapAdmin(username, password string) error {
	if len(password) < 12 {
		return errors.New("password must be at least 12 characters")
	}
	username = normalizeUsername(username)
	if username == "" {
		return errors.New("username is required")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	cred := &credential{Username: username, Salt: salt, Hash: pbkdf2([]byte(password), salt, pbkdf2Iterations, keyLen)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admin != nil {
		return errors.New("administrator already provisioned")
	}
	s.admin = cred
	s.saveLocked()
	return nil
}

// Login verifies credentials and issues a session; the returned value is the
// raw cookie/session identifier.
func (s *Store) Login(username, password string) (string, time.Time, error) {
	cred := func() *credential {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.admin
	}()
	// Always run a derivation to keep timing uniform whether or not the
	// administrator exists.
	salt := make([]byte, 16)
	if cred != nil {
		copy(salt, cred.Salt)
	}
	candidate := pbkdf2([]byte(password), salt, pbkdf2Iterations, keyLen)
	if cred == nil ||
		subtle.ConstantTimeCompare([]byte(normalizeUsername(username)), []byte(cred.Username)) != 1 ||
		subtle.ConstantTimeCompare(candidate, cred.Hash) != 1 {
		return "", time.Time{}, errors.New("invalid credentials")
	}
	return s.newSession(cred.Username, RoleAdmin)
}

func (s *Store) newSession(subject, role string) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	id := hex.EncodeToString(raw)
	expires := s.now().Add(s.sessionTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = &session{id: id, subject: subject, role: role, expiresAt: expires}
	return id, expires, nil
}

// IssueToken verifies credentials (or an existing principal's session) and
// mints a bearer token. The raw token is returned exactly once; only its
// SHA-256 digest is stored.
func (s *Store) IssueToken(username, password string) (string, time.Time, error) {
	if _, _, err := s.Login(username, password); err != nil {
		return "", time.Time{}, err
	}
	return s.mintToken(normalizeUsername(username), RoleAdmin)
}

// IssueTokenForSession mints a bearer token for an authenticated session.
func (s *Store) IssueTokenForSession(sessionID string) (string, time.Time, error) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return "", time.Time{}, errors.New("session not found")
	}
	if s.now().After(sess.expiresAt) {
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return "", time.Time{}, errors.New("session expired")
	}
	subject, role := sess.subject, sess.role
	sess.expiresAt = s.now().Add(s.sessionTTL) // sliding expiry
	s.mu.Unlock()
	return s.mintToken(subject, role)
}

func (s *Store) mintToken(subject, role string) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(raw)
	expires := s.now().Add(s.tokenTTL)
	sum := sha256.Sum256([]byte(token))
	rec := &apiToken{hash: sum, subject: subject, role: role, expiresAt: expires}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[hex.EncodeToString(sum[:])] = rec
	s.saveLocked()
	return token, expires, nil
}

// Logout destroys a session.
func (s *Store) Logout(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// Authenticate resolves the principal behind a session cookie value.
func (s *Store) AuthenticateSession(id string) (Principal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Principal{}, false
	}
	now := s.now()
	if now.After(sess.expiresAt) {
		delete(s.sessions, id)
		return Principal{}, false
	}
	sess.expiresAt = now.Add(s.sessionTTL) // sliding expiry
	return Principal{Subject: sess.subject, Role: sess.role, Via: "session"}, true
}

// AuthenticateToken resolves the principal behind a bearer token value.
// Only digests are compared, always constant-time.
func (s *Store) AuthenticateToken(raw string) (Principal, bool) {
	sum := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, ok := s.tokens[key]
	if !ok {
		return Principal{}, false
	}
	stored, _ := hex.DecodeString(key)
	if subtle.ConstantTimeCompare(stored, tok.hash[:]) != 1 {
		return Principal{}, false
	}
	if s.now().After(tok.expiresAt) {
		delete(s.tokens, key)
		return Principal{}, false
	}
	return Principal{Subject: tok.subject, Role: tok.role, Via: "bearer"}, true
}

func normalizeUsername(u string) string {
	out := make([]byte, 0, len(u))
	for _, r := range u {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		out = append(out, []byte(string(r))...)
	}
	return string(out)
}

// pbkdf2 implements PBKDF2-HMAC-SHA256 (RFC 8018) on stdlib primitives only.
func pbkdf2(password, salt []byte, iterations, length int) []byte {
	mac := hmac.New(sha256.New, password)
	hashLen := mac.Size()
	blocks := (length + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	buf := make([]byte, 4)
	for block := 1; block <= blocks; block++ {
		mac.Reset()
		mac.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		mac.Write(buf)
		u := mac.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iterations; i++ {
			mac.Reset()
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:length]
}

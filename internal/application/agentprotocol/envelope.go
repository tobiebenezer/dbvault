package agentprotocol

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Envelope struct {
	Version   int              `json:"version"`
	Task      domain.AgentTask `json:"task"`
	KeyID     string           `json:"key_id"`
	Signature string           `json:"signature"`
}

type ReplayProtector interface {
	Accept(agentID domain.AgentID, nonce string, expires time.Time) bool
}

type MemoryReplayProtector struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func NewMemoryReplayProtector() *MemoryReplayProtector {
	return &MemoryReplayProtector{seen: map[string]time.Time{}}
}
func (r *MemoryReplayProtector) Accept(agentID domain.AgentID, nonce string, expires time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for key, expiry := range r.seen {
		if expiry.Before(now) {
			delete(r.seen, key)
		}
	}
	key := string(agentID) + ":" + nonce
	if _, ok := r.seen[key]; ok {
		return false
	}
	r.seen[key] = expires
	return true
}

func Sign(task domain.AgentTask, keyID string, private ed25519.PrivateKey) (Envelope, error) {
	if len(private) != ed25519.PrivateKeySize {
		return Envelope{}, errors.New("invalid private key")
	}
	payload, err := canonical(task)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Version: 1, Task: task, KeyID: keyID, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))}, nil
}

func Verify(envelope Envelope, expectedAgent domain.AgentID, public ed25519.PublicKey, now time.Time, replay ReplayProtector) error {
	if envelope.Version != 1 {
		return errors.New("unsupported task envelope version")
	}
	if envelope.Task.AgentID != expectedAgent {
		return errors.New("task is addressed to another agent")
	}
	if envelope.Task.IssuedAt.After(now.Add(2 * time.Minute)) {
		return errors.New("task issued in the future")
	}
	if !envelope.Task.ExpiresAt.After(now) {
		return errors.New("task expired")
	}
	sig, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return err
	}
	payload, err := canonical(envelope.Task)
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, payload, sig) {
		return errors.New("invalid task signature")
	}
	if replay != nil && !replay.Accept(expectedAgent, envelope.Task.Nonce, envelope.Task.ExpiresAt) {
		return errors.New("replayed task")
	}
	return nil
}

func canonical(task domain.AgentTask) ([]byte, error) { return json.Marshal(task) }

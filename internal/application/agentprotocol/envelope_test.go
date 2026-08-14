package agentprotocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestSignedTaskAndReplayProtection(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	task := domain.AgentTask{ID: "task-1", OrganisationID: "org", AgentID: "agent", Type: "backup", IssuedAt: now, ExpiresAt: now.Add(time.Minute), Nonce: "nonce-1"}
	env, err := Sign(task, "controller", priv)
	if err != nil {
		t.Fatal(err)
	}
	replay := NewMemoryReplayProtector()
	if err := Verify(env, "agent", pub, now, replay); err != nil {
		t.Fatal(err)
	}
	if err := Verify(env, "agent", pub, now, replay); err == nil {
		t.Fatal("expected replay rejection")
	}
}

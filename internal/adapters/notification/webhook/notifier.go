// Package webhook delivers dbvault events to an HTTP endpoint as HMAC-signed
// JSON POSTs. Delivery failures are always returned as errors, never fatal to
// the caller's pipeline; callers log them.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"github.com/dbvault/dbvault/internal/ports"
)

const SignatureHeader = "X-DBVault-Signature"

// Backoff schedule: 1s, 4s, 16s ... capped at 30s, jittered ±25%.
const (
	defaultAttempts = 3
	backoffBase     = time.Second
	backoffFactor   = 4
	backoffCap      = 30 * time.Second
	defaultTimeout  = 10 * time.Second
	signaturePrefix = "sha256="
)

type Notifier struct {
	URL         string
	Secret      string
	Client      *http.Client
	MaxAttempts int

	// Sleep is overridden in tests to observe retries without real delays.
	Sleep func(context.Context, time.Duration) error
}

// New builds a notifier for the resolved endpoint URL and signing secret.
// An empty secret disables signing rather than delivery.
func New(url, secret string) *Notifier {
	return &Notifier{URL: url, Secret: secret}
}

func (n *Notifier) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return &http.Client{Timeout: defaultTimeout}
}

func (n *Notifier) attempts() int {
	if n.MaxAttempts > 0 {
		return n.MaxAttempts
	}
	return defaultAttempts
}

func (n *Notifier) sleep(ctx context.Context, d time.Duration) error {
	if n.Sleep != nil {
		return n.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (n *Notifier) Notify(ctx context.Context, event ports.Event) error {
	if n.URL == "" {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"id":         event.ID,
		"event":      event.Type,
		"severity":   event.Severity,
		"resource":   event.Resource,
		"metadata":   event.Metadata,
		"created_at": event.CreatedAt.UTC().Format(time.RFC3339Nano),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("webhook marshal payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < n.attempts(); attempt++ {
		if attempt > 0 {
			if err := n.sleep(ctx, backoff(attempt)); err != nil {
				return err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "DBVault-Webhook/1.0")
		if n.Secret != "" {
			mac := hmac.New(sha256.New, []byte(n.Secret))
			mac.Write(body)
			req.Header.Set(SignatureHeader, signaturePrefix+hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := n.client().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
	}
	slog.Warn("webhook delivery failed", "url_host_redacted", true, "event", event.Type, "attempts", n.attempts(), "error", lastErr)
	return lastErr
}

// VerifySignature recomputes the HMAC over body and reports whether the
// header value matches. Exported for receiver-side validation tooling.
func VerifySignature(secret string, body []byte, header string) bool {
	if len(header) <= len(signaturePrefix) || header[:len(signaturePrefix)] != signaturePrefix {
		return false
	}
	got, err := hex.DecodeString(header[len(signaturePrefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

func backoff(attempt int) time.Duration {
	d := backoffBase
	for i := 1; i < attempt; i++ {
		d *= backoffFactor
		if d >= backoffCap {
			return jitter(backoffCap)
		}
	}
	return jitter(d)
}

// jitter widens a delay by ±25% so fleet-wide retries do not synchronize.
func jitter(d time.Duration) time.Duration {
	n, err := rand.Int(rand.Reader, big.NewInt(50))
	if err != nil {
		return d
	}
	factor := 75 + int(n.Int64()) // 75..124 => 0.75x..1.24x
	return d * time.Duration(factor) / 100
}

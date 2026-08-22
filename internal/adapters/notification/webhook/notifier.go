package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Notifier struct {
	URL        string
	SecretKey  string
	Client     *http.Client
	MaxRetries int
}

func (n Notifier) Notify(ctx context.Context, event string, payload any) error {
	if n.URL == "" {
		return nil
	}
	c := n.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	maxRetries := n.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	body, err := json.Marshal(map[string]any{
		"event":        event,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"payload":      payload,
		"appliance_id": "dbvault-appliance",
	})
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt*500) * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "DBVault-Webhook/1.0")

		if n.SecretKey != "" {
			mac := hmac.New(sha256.New, []byte(n.SecretKey))
			mac.Write(body)
			req.Header.Set("X-DBVault-Signature", hex.EncodeToString(mac.Sum(nil)))
		}

		resp, err := c.Do(req)
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

	return lastErr
}

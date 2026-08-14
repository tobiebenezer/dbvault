package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Notifier struct {
	URL    string
	Client *http.Client
}

func (n Notifier) Notify(ctx context.Context, event string, payload any) error {
	if n.URL == "" {
		return nil
	}
	c := n.Client
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	body, _ := json.Marshal(map[string]any{"event": event, "payload": payload})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

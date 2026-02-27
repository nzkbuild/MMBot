package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"mmbot/internal/domain"
)

type Client struct {
	webhookURL string
	timeout    time.Duration
	httpClient *http.Client
}

func NewClient(webhookURL string, timeout time.Duration) *Client {
	return &Client{
		webhookURL: webhookURL,
		timeout:    timeout,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Publish(ctx context.Context, event domain.Event) error {
	if c.webhookURL == "" {
		return nil
	}

	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", event.ID)
	req.Header.Set("X-Event-Type", string(event.Type))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}


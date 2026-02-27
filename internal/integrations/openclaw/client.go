package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"mmbot/internal/domain"
)

type Client struct {
	webhookURL string
	timeout    time.Duration
	maxRetries int
	retryBase  time.Duration
	retryMax   time.Duration
	httpClient *http.Client
}

func NewClient(webhookURL string, timeout time.Duration, maxRetries int, retryBase, retryMax time.Duration) *Client {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if retryBase <= 0 {
		retryBase = 500 * time.Millisecond
	}
	if retryMax < retryBase {
		retryMax = retryBase
	}
	return &Client{
		webhookURL: webhookURL,
		timeout:    timeout,
		maxRetries: maxRetries,
		retryBase:  retryBase,
		retryMax:   retryMax,
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
	totalAttempts := 1 + c.maxRetries
	var lastErr error

	for attempt := 1; attempt <= totalAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Event-ID", event.ID)
		req.Header.Set("X-Event-Type", string(event.Type))
		req.Header.Set("X-Idempotency-Key", event.ID)
		req.Header.Set("X-Delivery-Attempt", fmt.Sprintf("%d", attempt))

		resp, err := c.httpClient.Do(req)
		if err == nil {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				return nil
			}
			lastErr = fmt.Errorf("openclaw http status=%d attempt=%d body=%s", resp.StatusCode, attempt, string(data))
		} else {
			lastErr = fmt.Errorf("openclaw request failed attempt=%d err=%w", attempt, err)
		}

		if attempt >= totalAttempts {
			break
		}
		wait := c.backoff(attempt)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return lastErr
}

func (c *Client) backoff(attempt int) time.Duration {
	// attempt starts at 1 for first retry interval calc
	multiplier := 1 << (attempt - 1)
	wait := time.Duration(multiplier) * c.retryBase
	if wait > c.retryMax {
		return c.retryMax
	}
	return wait
}

package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Notifier struct {
	botToken string
	chatID   string
	client   *http.Client
}

func NewNotifier(botToken, chatID string) *Notifier {
	return &Notifier{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (n *Notifier) Notify(ctx context.Context, text string) error {
	return n.NotifyChat(ctx, n.chatID, text)
}

func (n *Notifier) NotifyChat(ctx context.Context, chatID, text string) error {
	if n.botToken == "" || chatID == "" || text == "" {
		return nil
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)

	body := map[string]string{
		"chat_id": chatID,
		"text":    text,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("telegram sendMessage failed: status=%d", resp.StatusCode)
	}
	return nil
}

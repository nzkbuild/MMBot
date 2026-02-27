package memory

import (
	"testing"
	"time"

	"mmbot/internal/domain"
)

func TestIssueAndValidateEASession(t *testing.T) {
	store := NewStore(24 * time.Hour)
	session := store.IssueEASession("paper-1", "device-1")
	if session.Token == "" {
		t.Fatal("expected token to be set")
	}
	_, err := store.ValidateEASession(session.Token)
	if err != nil {
		t.Fatalf("expected token to validate, got error: %v", err)
	}
}

func TestQueueAndDispatchCommand(t *testing.T) {
	store := NewStore(24 * time.Hour)
	store.EnqueueCommand(domain.Command{
		AccountID: "paper-1",
		Type:      domain.CommandOpen,
		Symbol:    "EURUSD",
		ExpiresAt: time.Now().Add(30 * time.Second),
	})
	cmd, err := store.NextQueuedCommand("paper-1")
	if err != nil {
		t.Fatalf("expected queued command, got error: %v", err)
	}
	if cmd.Status != domain.CommandStatusDispatched {
		t.Fatalf("expected dispatched status, got %s", cmd.Status)
	}
}

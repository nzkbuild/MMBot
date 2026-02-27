package openclaw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mmbot/internal/domain"
)

func TestPublishRetriesAndSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if r.Header.Get("X-Idempotency-Key") != "evt-1" {
			t.Fatalf("missing idempotency header")
		}
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream error"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 2*time.Second, 3, 5*time.Millisecond, 20*time.Millisecond)
	err := client.Publish(context.Background(), domain.Event{
		ID:   "evt-1",
		Type: domain.EventSignalProposed,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestPublishFailsAfterMaxRetries(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 2*time.Second, 2, 5*time.Millisecond, 20*time.Millisecond)
	err := client.Publish(context.Background(), domain.Event{
		ID:   "evt-fail",
		Type: domain.EventSignalProposed,
	})
	if err == nil {
		t.Fatalf("expected failure, got nil")
	}
	if atomic.LoadInt32(&attempts) != 3 { // initial + 2 retries
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

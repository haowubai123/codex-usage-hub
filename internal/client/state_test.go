package client

import (
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"codex-usage-tracker/internal/shared"
)

func TestLoadOrCreateDeviceIDIsUUIDLikeAndStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	first, err := LoadOrCreateDeviceID(path)
	if err != nil {
		t.Fatalf("LoadOrCreateDeviceID(first) error = %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(first) {
		t.Fatalf("device id %q is not UUID-like", first)
	}

	second, err := LoadOrCreateDeviceID(path)
	if err != nil {
		t.Fatalf("LoadOrCreateDeviceID(second) error = %v", err)
	}
	if second != first {
		t.Fatalf("second device id = %q, want stable %q", second, first)
	}
}

func TestPendingQueuePersistsFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.json")
	events := []shared.UsageEvent{
		{EventID: "event-1", DeviceID: "device", IdentityKey: "alice", Source: "codex", Model: "gpt-5", InputTokens: 1, OccurredAt: time.Unix(1, 0).UTC()},
		{EventID: "event-2", DeviceID: "device", IdentityKey: "alice", Source: "codex", Model: "gpt-5", InputTokens: 2, OccurredAt: time.Unix(2, 0).UTC()},
	}
	more := []shared.UsageEvent{
		{EventID: "event-3", DeviceID: "device", IdentityKey: "alice", Source: "codex", Model: "gpt-5", InputTokens: 3, OccurredAt: time.Unix(3, 0).UTC()},
	}

	if err := EnqueuePending(path, events); err != nil {
		t.Fatalf("EnqueuePending(events) error = %v", err)
	}
	if err := EnqueuePending(path, more); err != nil {
		t.Fatalf("EnqueuePending(more) error = %v", err)
	}

	got, err := LoadPending(path)
	if err != nil {
		t.Fatalf("LoadPending() error = %v", err)
	}
	wantIDs := []string{"event-1", "event-2", "event-3"}
	if len(got) != len(wantIDs) {
		t.Fatalf("LoadPending() len = %d, want %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].EventID != want {
			t.Fatalf("event %d id = %q, want %q", i, got[i].EventID, want)
		}
	}

	if err := ReplacePending(path, got[1:]); err != nil {
		t.Fatalf("ReplacePending() error = %v", err)
	}
	got, err = LoadPending(path)
	if err != nil {
		t.Fatalf("LoadPending(after replace) error = %v", err)
	}
	wantIDs = []string{"event-2", "event-3"}
	if len(got) != len(wantIDs) {
		t.Fatalf("LoadPending(after replace) len = %d, want %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].EventID != want {
			t.Fatalf("after replace event %d id = %q, want %q", i, got[i].EventID, want)
		}
	}
}

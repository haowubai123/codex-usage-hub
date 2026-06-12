package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannerEmitsTokenDeltasAndPersistsCursor(t *testing.T) {
	codexHome := t.TempDir()
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "06", "13")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(sessionDir) error = %v", err)
	}

	fixture, err := os.ReadFile(filepath.Join("testdata", "codex-session.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile(fixture) error = %v", err)
	}
	sessionPath := filepath.Join(sessionDir, "codex-session.jsonl")
	if err := os.WriteFile(sessionPath, fixture, 0o600); err != nil {
		t.Fatalf("WriteFile(session) error = %v", err)
	}

	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	scanner := Scanner{
		CodexHome:   codexHome,
		DeviceID:    "device-1",
		IdentityKey: "person-a",
		Now:         func() time.Time { return now },
	}
	state := &ScanState{Files: map[string]FileCursor{}}

	events, err := scanner.Scan(state)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("Scan() emitted %d events, want 2", len(events))
	}

	first := events[0]
	if first.DeviceID != "device-1" {
		t.Fatalf("first DeviceID = %q, want device-1", first.DeviceID)
	}
	if first.IdentityKey != "person-a" {
		t.Fatalf("first IdentityKey = %q, want person-a", first.IdentityKey)
	}
	if first.SessionID != "session-123" {
		t.Fatalf("first SessionID = %q, want session-123", first.SessionID)
	}
	if first.Source != "codex" {
		t.Fatalf("first Source = %q, want codex", first.Source)
	}
	if first.Model != "gpt-5.2-codex-low" {
		t.Fatalf("first Model = %q, want gpt-5.2-codex-low", first.Model)
	}
	if first.InputTokens != 1000 || first.CacheReadTokens != 250 || first.OutputTokens != 100 {
		t.Fatalf("first tokens = input %d cache %d output %d, want 1000/250/100", first.InputTokens, first.CacheReadTokens, first.OutputTokens)
	}

	second := events[1]
	if second.IdentityKey != "person-a" {
		t.Fatalf("second IdentityKey = %q, want person-a", second.IdentityKey)
	}
	if second.InputTokens != 600 || second.CacheReadTokens != 50 || second.OutputTokens != 120 {
		t.Fatalf("second tokens = input %d cache %d output %d, want 600/50/120", second.InputTokens, second.CacheReadTokens, second.OutputTokens)
	}
	if second.EventID == "" {
		t.Fatal("second EventID is empty")
	}
	if second.EventID == first.EventID {
		t.Fatal("event IDs must be unique per usage event")
	}

	cursor, ok := state.Files[sessionPath]
	if !ok {
		t.Fatalf("state cursor missing for %s", sessionPath)
	}
	if cursor.SessionID != "session-123" || cursor.Model != "gpt-5.2-codex-low" {
		t.Fatalf("cursor session/model = %q/%q, want session-123/gpt-5.2-codex-low", cursor.SessionID, cursor.Model)
	}
	if cursor.EventIndex != 2 {
		t.Fatalf("cursor EventIndex = %d, want 2", cursor.EventIndex)
	}
	if cursor.PrevInput != 1600 || cursor.PrevCacheRead != 300 || cursor.PrevOutput != 220 {
		t.Fatalf("cursor prev = input %d cache %d output %d, want 1600/300/220", cursor.PrevInput, cursor.PrevCacheRead, cursor.PrevOutput)
	}
	if cursor.LineOffset != 5 {
		t.Fatalf("cursor LineOffset = %d, want 5", cursor.LineOffset)
	}

	again, err := scanner.Scan(state)
	if err != nil {
		t.Fatalf("Scan(rescan) error = %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("Scan(rescan) emitted %d events, want 0", len(again))
	}
}

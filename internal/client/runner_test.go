package client

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"codex-usage-tracker/internal/shared"
)

func TestRunnerUploadsPendingBeforeNewlyScannedEvents(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	queuePath := filepath.Join(dir, "pending.json")
	state := ScanState{DeviceID: "device-1", Files: map[string]FileCursor{}}
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	pending := []shared.UsageEvent{testUsageEvent("pending-1")}
	if err := ReplacePending(queuePath, pending); err != nil {
		t.Fatalf("ReplacePending() error = %v", err)
	}

	scanner := &fakeRunnerScanner{events: []shared.UsageEvent{testUsageEvent("scanned-1")}, consume: true}
	uploader := &fakeRunnerUploader{}
	runner := Runner{
		Config:       ClientConfig{IdentityKey: "person-a", ScanInterval: time.Minute},
		StatePath:    statePath,
		QueuePath:    queuePath,
		ScannerImpl:  scanner,
		UploaderImpl: uploader,
	}

	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(uploader.requests) != 2 {
		t.Fatalf("upload calls = %d, want 2", len(uploader.requests))
	}
	if uploader.requests[0].Events[0].EventID != "pending-1" {
		t.Fatalf("first upload event = %q, want pending-1", uploader.requests[0].Events[0].EventID)
	}
	if uploader.requests[1].Events[0].EventID != "scanned-1" {
		t.Fatalf("second upload event = %q, want scanned-1", uploader.requests[1].Events[0].EventID)
	}
	if scanner.calls != 1 {
		t.Fatalf("scanner calls = %d, want 1", scanner.calls)
	}
}

func TestRunnerUploadFailureLeavesEventsPending(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	queuePath := filepath.Join(dir, "pending.json")
	state := ScanState{DeviceID: "device-1", Files: map[string]FileCursor{}}
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	wantErr := errors.New("temporary outage")
	scanner := &fakeRunnerScanner{events: []shared.UsageEvent{testUsageEvent("scanned-1")}, consume: true}
	uploader := &fakeRunnerUploader{err: wantErr}
	runner := Runner{
		Config:       ClientConfig{IdentityKey: "person-a", ScanInterval: time.Minute},
		StatePath:    statePath,
		QueuePath:    queuePath,
		ScannerImpl:  scanner,
		UploaderImpl: uploader,
	}

	err := runner.RunOnce(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want %v", err, wantErr)
	}
	pending, loadErr := LoadPending(queuePath)
	if loadErr != nil {
		t.Fatalf("LoadPending() error = %v", loadErr)
	}
	if len(pending) != 1 || pending[0].EventID != "scanned-1" {
		t.Fatalf("pending = %#v, want scanned-1", pending)
	}
}

func TestRunnerSuccessfulUploadClearsAcceptedEvents(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	queuePath := filepath.Join(dir, "pending.json")
	state := ScanState{DeviceID: "device-1", Files: map[string]FileCursor{}}
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	pending := []shared.UsageEvent{testUsageEvent("pending-1"), testUsageEvent("pending-2")}
	if err := ReplacePending(queuePath, pending); err != nil {
		t.Fatalf("ReplacePending() error = %v", err)
	}

	scanner := &fakeRunnerScanner{}
	uploader := &fakeRunnerUploader{responses: []shared.IngestResponse{{Accepted: 1}}}
	runner := Runner{
		Config:       ClientConfig{IdentityKey: "person-a", ScanInterval: time.Minute},
		StatePath:    statePath,
		QueuePath:    queuePath,
		ScannerImpl:  scanner,
		UploaderImpl: uploader,
	}

	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	pending, err := LoadPending(queuePath)
	if err != nil {
		t.Fatalf("LoadPending() error = %v", err)
	}
	if len(pending) != 1 || pending[0].EventID != "pending-2" {
		t.Fatalf("pending = %#v, want only pending-2", pending)
	}
	if scanner.calls != 0 {
		t.Fatalf("scanner calls = %d, want 0 while queue still has pending events", scanner.calls)
	}
}

func TestRunnerRunForeverContinuesAfterRetryableError(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	queuePath := filepath.Join(dir, "pending.json")
	state := ScanState{DeviceID: "device-1", Files: map[string]FileCursor{}}
	if err := SaveState(statePath, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wantErr := errors.New("temporary outage")
	scanner := &fakeRunnerScanner{events: []shared.UsageEvent{testUsageEvent("scanned-1")}, consume: true}
	uploader := &fakeRunnerUploader{
		errs: []error{wantErr, nil},
		afterUpload: func(calls int) {
			if calls == 2 {
				cancel()
			}
		},
	}
	runner := Runner{
		Config:       ClientConfig{IdentityKey: "person-a", ScanInterval: time.Millisecond},
		StatePath:    statePath,
		QueuePath:    queuePath,
		ScannerImpl:  scanner,
		UploaderImpl: uploader,
	}

	err := runner.RunForever(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunForever() error = %v, want context.Canceled", err)
	}
	if len(uploader.requests) != 2 {
		t.Fatalf("upload calls = %d, want 2", len(uploader.requests))
	}
	pending, loadErr := LoadPending(queuePath)
	if loadErr != nil {
		t.Fatalf("LoadPending() error = %v", loadErr)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %#v, want empty after retry succeeds", pending)
	}
}

func testUsageEvent(eventID string) shared.UsageEvent {
	return shared.UsageEvent{
		EventID:     eventID,
		DeviceID:    "device-1",
		IdentityKey: "person-a",
		Source:      "codex_session",
		Model:       "gpt-5",
		InputTokens: 1,
		OccurredAt:  time.Unix(1, 0).UTC(),
	}
}

type fakeRunnerScanner struct {
	calls   int
	events  []shared.UsageEvent
	err     error
	consume bool
}

func (s *fakeRunnerScanner) Scan(state *ScanState) ([]shared.UsageEvent, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	events := s.events
	if s.consume {
		s.events = nil
	}
	return events, nil
}

type fakeRunnerUploader struct {
	requests    []shared.IngestRequest
	responses   []shared.IngestResponse
	err         error
	errs        []error
	afterUpload func(calls int)
}

func (u *fakeRunnerUploader) Upload(ctx context.Context, req shared.IngestRequest) (shared.IngestResponse, error) {
	if err := ctx.Err(); err != nil {
		return shared.IngestResponse{}, err
	}
	u.requests = append(u.requests, req)
	if u.afterUpload != nil {
		u.afterUpload(len(u.requests))
	}
	if len(u.errs) > 0 {
		err := u.errs[0]
		u.errs = u.errs[1:]
		if err != nil {
			return shared.IngestResponse{}, err
		}
	}
	if u.err != nil {
		return shared.IngestResponse{}, u.err
	}
	if len(u.responses) == 0 {
		return shared.IngestResponse{Accepted: len(req.Events)}, nil
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

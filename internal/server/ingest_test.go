package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"codex-usage-tracker/internal/shared"
)

func TestIngestHandlerRequiresBearerToken(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	srv.IngestHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestIngestHandlerRejectsWrongBearerToken(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()

	srv.IngestHandler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestIngestHandlerInsertsEventOnceAndCountsDuplicates(t *testing.T) {
	srv := newTestServer(t)
	event := testUsageEvent("evt-1", time.Date(2026, 6, 13, 10, 15, 0, 0, time.UTC))

	resp := postIngest(t, srv, shared.IngestRequest{Events: []shared.UsageEvent{event}})
	if resp.Accepted != 1 || resp.Duplicates != 0 {
		t.Fatalf("first response = %+v, want accepted 1 duplicates 0", resp)
	}

	resp = postIngest(t, srv, shared.IngestRequest{Events: []shared.UsageEvent{event}})
	if resp.Accepted != 0 || resp.Duplicates != 1 {
		t.Fatalf("second response = %+v, want accepted 0 duplicates 1", resp)
	}

	var count int
	if err := srv.DB.QueryRow("SELECT COUNT(*) FROM usage_events WHERE event_id = ?", event.EventID).Scan(&count); err != nil {
		t.Fatalf("query event count: %v", err)
	}
	if count != 1 {
		t.Fatalf("stored event count = %d, want 1", count)
	}
}

func TestIngestHandlerStoresMissingPriceWithNullCost(t *testing.T) {
	srv := newTestServer(t)
	event := testUsageEvent("evt-missing", time.Date(2026, 6, 13, 10, 15, 0, 0, time.UTC))
	event.Model = "unknown-model-20260613"

	resp := postIngest(t, srv, shared.IngestRequest{Events: []shared.UsageEvent{event}})
	if resp.Accepted != 1 || resp.Duplicates != 0 {
		t.Fatalf("response = %+v, want accepted 1 duplicates 0", resp)
	}
	if len(resp.MissingPriceModels) != 1 || resp.MissingPriceModels[0] != "unknown-model" {
		t.Fatalf("MissingPriceModels = %+v, want [unknown-model]", resp.MissingPriceModels)
	}

	var status string
	var cost sql.NullFloat64
	if err := srv.DB.QueryRow("SELECT pricing_status, cost_usd FROM usage_events WHERE event_id = ?", event.EventID).Scan(&status, &cost); err != nil {
		t.Fatalf("query stored event: %v", err)
	}
	if status != "missing" {
		t.Fatalf("pricing_status = %q, want missing", status)
	}
	if cost.Valid {
		t.Fatalf("cost_usd = %v, want NULL", cost.Float64)
	}
}

func TestIngestHandlerUpdatesDeviceLastSeen(t *testing.T) {
	srv := newTestServer(t)
	first := testUsageEvent("evt-device-1", time.Date(2026, 6, 13, 10, 15, 0, 0, time.UTC))
	second := testUsageEvent("evt-device-2", time.Date(2026, 6, 13, 12, 45, 0, 0, time.UTC))

	postIngest(t, srv, shared.IngestRequest{Events: []shared.UsageEvent{first}})
	postIngest(t, srv, shared.IngestRequest{Events: []shared.UsageEvent{second}})

	var firstSeen, lastSeen string
	if err := srv.DB.QueryRow("SELECT first_seen_at, last_seen_at FROM devices WHERE device_id = ?", first.DeviceID).Scan(&firstSeen, &lastSeen); err != nil {
		t.Fatalf("query device: %v", err)
	}
	if firstSeen != first.OccurredAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("first_seen_at = %q, want %q", firstSeen, first.OccurredAt.UTC().Format(time.RFC3339Nano))
	}
	if lastSeen != second.OccurredAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("last_seen_at = %q, want %q", lastSeen, second.OccurredAt.UTC().Format(time.RFC3339Nano))
	}
}

func newTestServer(t *testing.T) Server {
	t.Helper()

	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := SeedModelPrices(ctx, db); err != nil {
		t.Fatalf("SeedModelPrices() error = %v", err)
	}

	return Server{DB: db, APIKey: "secret"}
}

func postIngest(t *testing.T, srv Server, reqBody shared.IngestRequest) shared.IngestResponse {
	t.Helper()

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.IngestHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp shared.IngestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func testUsageEvent(eventID string, occurredAt time.Time) shared.UsageEvent {
	return shared.UsageEvent{
		EventID:             eventID,
		DeviceID:            "device-1",
		IdentityKey:         "identity-1",
		SessionID:           "session-1",
		Source:              "codex-jsonl",
		Model:               "OpenAI/GPT-5.5-2026-05-14",
		InputTokens:         1000,
		CacheReadTokens:     200,
		CacheCreationTokens: 0,
		OutputTokens:        500,
		OccurredAt:          occurredAt,
	}
}

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codex-usage-tracker/internal/shared"
)

func TestSummaryHandlerReturnsFilteredBucketsAndTotals(t *testing.T) {
	srv := newQueryTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/summary?bucket=hour&identity_key=identity-1&from=2026-06-13T10:00:00Z&to=2026-06-13T12:00:00Z", nil)
	rec := httptest.NewRecorder()

	srv.SummaryHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp SummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Bucket != "hour" {
		t.Fatalf("Bucket = %q, want hour", resp.Bucket)
	}
	if len(resp.Buckets) != 2 {
		t.Fatalf("len(Buckets) = %d, want 2: %+v", len(resp.Buckets), resp.Buckets)
	}
	if resp.Buckets[0].BucketStart != time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("first bucket = %s, want 2026-06-13T10:00:00Z", resp.Buckets[0].BucketStart)
	}
	if resp.Totals.TotalTokens != 3820 || resp.Totals.EventCount != 3 {
		t.Fatalf("Totals = %+v, want total_tokens 3820 event_count 3", resp.Totals)
	}
	if resp.Totals.MissingPriceCount != 1 {
		t.Fatalf("MissingPriceCount = %d, want 1", resp.Totals.MissingPriceCount)
	}
	if resp.Totals.CostUSD == nil || *resp.Totals.CostUSD <= 0 {
		t.Fatalf("CostUSD = %v, want positive priced cost", resp.Totals.CostUSD)
	}
}

func TestBreakdownHandlerGroupsByIdentityDeviceAndModel(t *testing.T) {
	srv := newQueryTestServer(t)

	cases := []struct {
		name    string
		groupBy string
		wantKey string
	}{
		{name: "identity", groupBy: "identity", wantKey: "identity-1"},
		{name: "device", groupBy: "device", wantKey: "device-1"},
		{name: "model", groupBy: "model", wantKey: "gpt-5.5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/breakdown?group_by="+tc.groupBy+"&from=2026-06-13T00:00:00Z&to=2026-06-15T00:00:00Z", nil)
			rec := httptest.NewRecorder()

			srv.BreakdownHandler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var resp BreakdownResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.GroupBy != tc.groupBy {
				t.Fatalf("GroupBy = %q, want %q", resp.GroupBy, tc.groupBy)
			}
			if len(resp.Items) == 0 {
				t.Fatalf("Items is empty")
			}
			if resp.Items[0].Key != tc.wantKey {
				t.Fatalf("first key = %q, want %q; items = %+v", resp.Items[0].Key, tc.wantKey, resp.Items)
			}
			if resp.Totals.EventCount != 5 {
				t.Fatalf("Totals.EventCount = %d, want 5", resp.Totals.EventCount)
			}
		})
	}
}

func TestEventsHandlerReturnsNewestLimitedEvents(t *testing.T) {
	srv := newQueryTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=2", nil)
	rec := httptest.NewRecorder()

	srv.EventsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp EventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(resp.Events))
	}
	if resp.Events[0].EventID != "evt-5" || resp.Events[1].EventID != "evt-4" {
		t.Fatalf("event order = %s, %s; want evt-5, evt-4", resp.Events[0].EventID, resp.Events[1].EventID)
	}
}

func newQueryTestServer(t *testing.T) Server {
	t.Helper()

	srv := newTestServer(t)
	events := []shared.UsageEvent{
		queryEvent("evt-1", "identity-1", "device-1", "OpenAI/GPT-5.5-2026-05-14", time.Date(2026, 6, 13, 10, 15, 0, 0, time.UTC), 1000, 200, 0, 500),
		queryEvent("evt-2", "identity-1", "device-1", "gpt-5.5", time.Date(2026, 6, 13, 10, 45, 0, 0, time.UTC), 600, 100, 20, 200),
		queryEvent("evt-3", "identity-1", "device-2", "unknown-model-20260613", time.Date(2026, 6, 13, 11, 30, 0, 0, time.UTC), 800, 100, 0, 300),
		queryEvent("evt-4", "identity-2", "device-3", "gpt-5", time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC), 300, 0, 0, 100),
		queryEvent("evt-5", "identity-2", "device-3", "gpt-5", time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC), 400, 0, 0, 150),
	}
	postIngest(t, srv, shared.IngestRequest{Platform: "test", Events: events})
	return srv
}

func queryEvent(eventID, identityKey, deviceID, model string, occurredAt time.Time, input, cacheRead, cacheCreation, output int64) shared.UsageEvent {
	return shared.UsageEvent{
		EventID:             eventID,
		DeviceID:            deviceID,
		IdentityKey:         identityKey,
		SessionID:           "session-" + eventID,
		Source:              "codex-jsonl",
		Model:               model,
		InputTokens:         input,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
		OutputTokens:        output,
		OccurredAt:          occurredAt,
	}
}

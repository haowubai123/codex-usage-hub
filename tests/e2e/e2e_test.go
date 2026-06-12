package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codex-usage-tracker/internal/client"
	usageServer "codex-usage-tracker/internal/server"
	"codex-usage-tracker/internal/shared"
)

func TestEndToEndIdentityRollups(t *testing.T) {
	ctx := context.Background()
	db, err := usageServer.OpenDB(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := usageServer.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := usageServer.SeedModelPrices(ctx, db); err != nil {
		t.Fatalf("SeedModelPrices() error = %v", err)
	}

	const apiKey = "test-api-key"
	httpServer := httptest.NewServer(usageServer.Server{DB: db, APIKey: apiKey}.Handler())
	t.Cleanup(httpServer.Close)

	scans := []struct {
		fixture     string
		deviceID    string
		identityKey string
	}{
		{fixture: "device-a-1.jsonl", deviceID: "device-a-1", identityKey: "a"},
		{fixture: "device-b-1.jsonl", deviceID: "device-b-1", identityKey: "b"},
		{fixture: "device-a-2.jsonl", deviceID: "device-a-2", identityKey: "a"},
	}

	for _, scan := range scans {
		codexHome := fixtureCodexHome(t, scan.fixture)
		scanner := client.Scanner{
			CodexHome:   codexHome,
			DeviceID:    scan.deviceID,
			IdentityKey: scan.identityKey,
			Now:         func() time.Time { return time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC) },
		}
		state := &client.ScanState{Files: map[string]client.FileCursor{}}
		events, err := scanner.Scan(state)
		if err != nil {
			t.Fatalf("Scan(%s) error = %v", scan.fixture, err)
		}
		if len(events) != 2 {
			t.Fatalf("Scan(%s) emitted %d events, want 2", scan.fixture, len(events))
		}

		resp, err := (client.Uploader{
			ServerURL: httpServer.URL,
			APIKey:    apiKey,
			Client:    httpServer.Client(),
		}).Upload(ctx, shared.IngestRequest{
			DeviceID:    scan.deviceID,
			IdentityKey: scan.identityKey,
			Platform:    "e2e",
			Events:      events,
		})
		if err != nil {
			t.Fatalf("Upload(%s) error = %v", scan.fixture, err)
		}
		if resp.Accepted != 2 || resp.Duplicates != 0 {
			t.Fatalf("Upload(%s) response accepted/duplicates = %d/%d, want 2/0", scan.fixture, resp.Accepted, resp.Duplicates)
		}
	}

	var summary usageServer.SummaryResponse
	getJSON(t, httpServer.Client(), httpServer.URL+"/api/v1/summary?bucket=day", &summary)
	if summary.Totals.EventCount != 6 {
		t.Fatalf("summary event_count = %d, want 6", summary.Totals.EventCount)
	}
	if summary.Totals.InputTokens != 450 || summary.Totals.CacheReadTokens != 90 || summary.Totals.OutputTokens != 150 || summary.Totals.TotalTokens != 690 {
		t.Fatalf("summary totals = input %d cache %d output %d total %d, want 450/90/150/690", summary.Totals.InputTokens, summary.Totals.CacheReadTokens, summary.Totals.OutputTokens, summary.Totals.TotalTokens)
	}

	var byIdentity usageServer.BreakdownResponse
	getJSON(t, httpServer.Client(), httpServer.URL+"/api/v1/breakdown?group_by=identity", &byIdentity)
	items := breakdownByKey(byIdentity)
	assertBreakdown(t, items, "a", 4, 330, 70, 115, 515)
	assertBreakdown(t, items, "b", 2, 120, 20, 35, 175)

	var byDevice usageServer.BreakdownResponse
	getJSON(t, httpServer.Client(), httpServer.URL+"/api/v1/breakdown?group_by=device", &byDevice)
	devices := breakdownByKey(byDevice)
	assertBreakdown(t, devices, "device-a-1", 2, 180, 40, 60, 280)
	assertBreakdown(t, devices, "device-b-1", 2, 120, 20, 35, 175)
	assertBreakdown(t, devices, "device-a-2", 2, 150, 30, 55, 235)
}

func fixtureCodexHome(t *testing.T, fixture string) string {
	t.Helper()
	source := filepath.Join("..", "fixtures", fixture)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", source, err)
	}

	codexHome := t.TempDir()
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "06", "13")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", sessionDir, err)
	}
	target := filepath.Join(sessionDir, fixture)
	if err := os.WriteFile(target, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", target, err)
	}
	return codexHome
}

func getJSON(t *testing.T, client *http.Client, url string, out any) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s error = %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("Decode(%s) error = %v", url, err)
	}
}

func breakdownByKey(resp usageServer.BreakdownResponse) map[string]usageServer.BreakdownItem {
	items := make(map[string]usageServer.BreakdownItem, len(resp.Items))
	for _, item := range resp.Items {
		items[item.Key] = item
	}
	return items
}

func assertBreakdown(t *testing.T, items map[string]usageServer.BreakdownItem, key string, eventCount, input, cacheRead, output, total int64) {
	t.Helper()
	item, ok := items[key]
	if !ok {
		t.Fatalf("breakdown item %q missing", key)
	}
	if item.EventCount != eventCount || item.InputTokens != input || item.CacheReadTokens != cacheRead || item.OutputTokens != output || item.TotalTokens != total {
		t.Fatalf("breakdown[%s] = events %d input %d cache %d output %d total %d, want %d/%d/%d/%d/%d", key, item.EventCount, item.InputTokens, item.CacheReadTokens, item.OutputTokens, item.TotalTokens, eventCount, input, cacheRead, output, total)
	}
}

package server

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"codex-usage-tracker/internal/shared"
)

func TestUpdateRollupsSumsHourAndDayBuckets(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	firstCost := 1.25
	secondCost := 2.75
	nextDayCost := 4.50
	events := []struct {
		event shared.UsageEvent
		cost  *float64
	}{
		{event: rollupEvent("evt-1", time.Date(2026, 6, 13, 10, 15, 0, 0, time.UTC), 100, 20, 5, 30), cost: &firstCost},
		{event: rollupEvent("evt-2", time.Date(2026, 6, 13, 10, 45, 0, 0, time.UTC), 200, 30, 7, 40), cost: &secondCost},
		{event: rollupEvent("evt-3", time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC), 300, 40, 11, 50), cost: &nextDayCost},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	for _, item := range events {
		if err := UpdateRollups(ctx, tx, item.event, item.cost); err != nil {
			tx.Rollback()
			t.Fatalf("UpdateRollups() error = %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	hour := queryRollup(t, db, "hour", time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC))
	assertRollup(t, hour, 300, 50, 12, 70, 432, 4.0, 2)

	day := queryRollup(t, db, "day", time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC))
	assertRollup(t, day, 300, 50, 12, 70, 432, 4.0, 2)

	nextDay := queryRollup(t, db, "day", time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC))
	assertRollup(t, nextDay, 300, 40, 11, 50, 401, 4.5, 1)
}

type rollupRow struct {
	inputTokens         int64
	cacheReadTokens     int64
	cacheCreationTokens int64
	outputTokens        int64
	totalTokens         int64
	cost                sql.NullFloat64
	eventCount          int64
}

func queryRollup(t *testing.T, db *sql.DB, granularity string, bucket time.Time) rollupRow {
	t.Helper()

	var row rollupRow
	err := db.QueryRow(`SELECT input_tokens, cache_read_tokens, cache_creation_tokens, output_tokens, total_tokens, cost_usd, event_count
		FROM usage_rollups
		WHERE bucket_granularity = ? AND bucket_start = ? AND identity_key = 'identity-1' AND device_id = 'device-1' AND model = 'gpt-5.5'`,
		granularity,
		bucket.Format(time.RFC3339Nano),
	).Scan(&row.inputTokens, &row.cacheReadTokens, &row.cacheCreationTokens, &row.outputTokens, &row.totalTokens, &row.cost, &row.eventCount)
	if err != nil {
		t.Fatalf("query rollup %s %s: %v", granularity, bucket, err)
	}
	return row
}

func assertRollup(t *testing.T, got rollupRow, inputTokens, cacheReadTokens, cacheCreationTokens, outputTokens, totalTokens int64, cost float64, eventCount int64) {
	t.Helper()

	if got.inputTokens != inputTokens || got.cacheReadTokens != cacheReadTokens || got.cacheCreationTokens != cacheCreationTokens || got.outputTokens != outputTokens || got.totalTokens != totalTokens || got.eventCount != eventCount {
		t.Fatalf("rollup tokens/count = %+v, want input=%d cacheRead=%d cacheCreation=%d output=%d total=%d count=%d", got, inputTokens, cacheReadTokens, cacheCreationTokens, outputTokens, totalTokens, eventCount)
	}
	if !got.cost.Valid || got.cost.Float64 != cost {
		t.Fatalf("rollup cost = %+v, want %v", got.cost, cost)
	}
}

func rollupEvent(eventID string, occurredAt time.Time, input, cacheRead, cacheCreation, output int64) shared.UsageEvent {
	return shared.UsageEvent{
		EventID:             eventID,
		DeviceID:            "device-1",
		IdentityKey:         "identity-1",
		Source:              "codex-jsonl",
		Model:               "OpenAI/GPT-5.5-2026-05-14",
		InputTokens:         input,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
		OutputTokens:        output,
		OccurredAt:          occurredAt,
	}
}

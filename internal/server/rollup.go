package server

import (
	"context"
	"database/sql"
	"time"

	"codex-usage-tracker/internal/shared"
)

func UpdateRollups(ctx context.Context, tx *sql.Tx, event shared.UsageEvent, cost *float64) error {
	event.Model = shared.NormalizeModel(event.Model)
	occurredAt := event.OccurredAt.UTC()
	buckets := []struct {
		start       time.Time
		granularity string
	}{
		{start: occurredAt.Truncate(time.Hour), granularity: "hour"},
		{start: time.Date(occurredAt.Year(), occurredAt.Month(), occurredAt.Day(), 0, 0, 0, 0, time.UTC), granularity: "day"},
	}

	totalTokens := event.InputTokens + event.CacheReadTokens + event.CacheCreationTokens + event.OutputTokens
	for _, bucket := range buckets {
		if err := upsertRollup(ctx, tx, bucket.start, bucket.granularity, event, totalTokens, cost); err != nil {
			return err
		}
	}
	return nil
}

func upsertRollup(ctx context.Context, tx *sql.Tx, bucketStart time.Time, granularity string, event shared.UsageEvent, totalTokens int64, cost *float64) error {
	var costValue any
	if cost != nil {
		costValue = *cost
	}

	_, err := tx.ExecContext(ctx, `INSERT INTO usage_rollups (
		bucket_start,
		bucket_granularity,
		identity_key,
		device_id,
		model,
		input_tokens,
		cache_read_tokens,
		cache_creation_tokens,
		output_tokens,
		total_tokens,
		cost_usd,
		event_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
	ON CONFLICT(bucket_start, bucket_granularity, identity_key, device_id, model) DO UPDATE SET
		input_tokens = input_tokens + excluded.input_tokens,
		cache_read_tokens = cache_read_tokens + excluded.cache_read_tokens,
		cache_creation_tokens = cache_creation_tokens + excluded.cache_creation_tokens,
		output_tokens = output_tokens + excluded.output_tokens,
		total_tokens = total_tokens + excluded.total_tokens,
		cost_usd = CASE
			WHEN usage_rollups.cost_usd IS NULL THEN excluded.cost_usd
			WHEN excluded.cost_usd IS NULL THEN usage_rollups.cost_usd
			ELSE usage_rollups.cost_usd + excluded.cost_usd
		END,
		event_count = event_count + 1`,
		bucketStart.Format(time.RFC3339Nano),
		granularity,
		event.IdentityKey,
		event.DeviceID,
		event.Model,
		event.InputTokens,
		event.CacheReadTokens,
		event.CacheCreationTokens,
		event.OutputTokens,
		totalTokens,
		costValue,
	)
	return err
}

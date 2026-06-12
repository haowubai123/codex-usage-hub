package server

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func OpenDB(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}

	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			device_id TEXT PRIMARY KEY,
			identity_key TEXT NOT NULL,
			label TEXT,
			platform TEXT,
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_events (
			event_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			identity_key TEXT NOT NULL,
			session_id TEXT,
			source TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			cache_read_tokens INTEGER NOT NULL,
			cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL,
			occurred_at TEXT NOT NULL,
			received_at TEXT NOT NULL,
			pricing_status TEXT NOT NULL,
			cost_usd REAL,
			FOREIGN KEY(device_id) REFERENCES devices(device_id)
		)`,
		`CREATE TABLE IF NOT EXISTS model_prices (
			model TEXT PRIMARY KEY,
			input_per_million REAL NOT NULL,
			cache_read_per_million REAL NOT NULL DEFAULT 0,
			cache_creation_per_million REAL NOT NULL DEFAULT 0,
			output_per_million REAL NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_rollups (
			bucket_start TEXT NOT NULL,
			bucket_granularity TEXT NOT NULL,
			identity_key TEXT NOT NULL,
			device_id TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			cache_read_tokens INTEGER NOT NULL,
			cache_creation_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			cost_usd REAL,
			event_count INTEGER NOT NULL,
			PRIMARY KEY(bucket_start, bucket_granularity, identity_key, device_id, model)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_identity_occurred_at ON usage_events(identity_key, occurred_at)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_device_occurred_at ON usage_events(device_id, occurred_at)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_model_occurred_at ON usage_events(model, occurred_at)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_rollups_granularity_start ON usage_rollups(bucket_granularity, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_rollups_identity_granularity_start ON usage_rollups(identity_key, bucket_granularity, bucket_start)`,
	}

	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func SeedModelPrices(ctx context.Context, db *sql.DB) error {
	const updatedAt = "builtin"
	prices := []struct {
		model                   string
		inputPerMillion         float64
		cacheReadPerMillion     float64
		cacheCreationPerMillion float64
		outputPerMillion        float64
	}{
		{model: "gpt-5", inputPerMillion: 5, cacheReadPerMillion: 0.5, cacheCreationPerMillion: 5, outputPerMillion: 15},
		{model: "gpt-5.5", inputPerMillion: 5, cacheReadPerMillion: 0.5, cacheCreationPerMillion: 5, outputPerMillion: 15},
		{model: "gpt-5.4", inputPerMillion: 5, cacheReadPerMillion: 0.5, cacheCreationPerMillion: 5, outputPerMillion: 15},
		{model: "gpt-5.2-codex-low", inputPerMillion: 1, cacheReadPerMillion: 0.1, cacheCreationPerMillion: 1, outputPerMillion: 3},
	}

	for _, price := range prices {
		_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO model_prices (
			model, input_per_million, cache_read_per_million, cache_creation_per_million, output_per_million, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
			price.model,
			price.inputPerMillion,
			price.cacheReadPerMillion,
			price.cacheCreationPerMillion,
			price.outputPerMillion,
			updatedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

package server

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenDBMigrateAndSeed(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "nested", "usage.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := SeedModelPrices(ctx, db); err != nil {
		t.Fatalf("SeedModelPrices() error = %v", err)
	}

	for _, table := range []string{"devices", "usage_events", "model_prices", "usage_rollups"} {
		if !tableExists(t, db, table) {
			t.Fatalf("table %q does not exist", table)
		}
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	for _, model := range []string{"gpt-5", "gpt-5.5", "gpt-5.4", "gpt-5.2-codex-low"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM model_prices WHERE model = ?", model).Scan(&count); err != nil {
			t.Fatalf("query seed price %q: %v", model, err)
		}
		if count != 1 {
			t.Fatalf("seed rows for %q = %d, want 1", model, count)
		}
	}
}

func TestDBSeedModelPricesPreservesExistingRows(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	const customPrice = 123.45
	_, err = db.ExecContext(ctx, `INSERT INTO model_prices (
		model, input_per_million, cache_read_per_million, cache_creation_per_million, output_per_million, updated_at
	) VALUES ('gpt-5', ?, 0, 0, 0, 'custom')`, customPrice)
	if err != nil {
		t.Fatalf("insert custom price: %v", err)
	}

	if err := SeedModelPrices(ctx, db); err != nil {
		t.Fatalf("SeedModelPrices() error = %v", err)
	}

	var got float64
	if err := db.QueryRowContext(ctx, "SELECT input_per_million FROM model_prices WHERE model = 'gpt-5'").Scan(&got); err != nil {
		t.Fatalf("query preserved price: %v", err)
	}
	if got != customPrice {
		t.Fatalf("input_per_million = %v, want preserved %v", got, customPrice)
	}
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count); err != nil {
		t.Fatalf("query table %q: %v", name, err)
	}
	return count == 1
}

package main

import (
	"context"
	"fmt"
	"os"

	"codex-usage-tracker/internal/server"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 || args[1] != "serve" {
		return usageError()
	}

	configPath := ""
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) {
				return usageError()
			}
			configPath = args[i+1]
			i++
		default:
			return usageError()
		}
	}
	if configPath == "" {
		return usageError()
	}

	cfg, err := server.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := server.ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	db, err := server.OpenDB(cfg.SQLitePath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := server.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	if err := server.SeedModelPrices(ctx, db); err != nil {
		return fmt.Errorf("seed model prices: %w", err)
	}

	fmt.Fprintf(os.Stderr, "codex-usage-server initialized %s; HTTP router is not implemented until Task 8\n", cfg.SQLitePath)
	return nil
}

func usageError() error {
	return fmt.Errorf("usage: codex-usage-server serve --config server.yaml")
}

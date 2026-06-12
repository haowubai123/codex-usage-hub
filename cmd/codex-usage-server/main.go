package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	if err := server.SeedModelPrices(ctx, db); err != nil {
		return fmt.Errorf("seed model prices: %w", err)
	}

	usageServer := server.Server{DB: db, APIKey: cfg.APIKey}
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           usageServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "codex-usage-server listening on %s\n", cfg.ListenAddr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func usageError() error {
	return fmt.Errorf("usage: codex-usage-server serve --config server.yaml")
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"codex-usage-tracker/internal/client"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: codex-usage-client <init|run|install-service|uninstall-service>")
	}

	switch args[0] {
	case "init":
		return initCommand(args[1:])
	case "run":
		return runCommand(args[1:])
	case "install-service", "uninstall-service":
		return fmt.Errorf("%s is not implemented yet", args[0])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func initCommand(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("usage: codex-usage-client init --config <path>")
	}

	if err := writeExampleConfigIfMissing(*configPath); err != nil {
		return err
	}
	deviceID, err := client.LoadOrCreateDeviceID(statePathForConfig(*configPath))
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "device_id: %s\n", deviceID)
	return nil
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("usage: codex-usage-client run --config <path>")
	}

	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if err := client.ValidateConfig(cfg); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	codexHome := client.ResolveCodexHome(cfg, map[string]string{"CODEX_HOME": os.Getenv("CODEX_HOME")}, home)
	statePath := statePathForConfig(*configPath)
	deviceID, err := client.LoadOrCreateDeviceID(statePath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := client.Runner{
		Config:    cfg,
		StatePath: statePath,
		QueuePath: queuePathForConfig(*configPath),
		Scanner: client.Scanner{
			CodexHome:   codexHome,
			DeviceID:    deviceID,
			IdentityKey: cfg.IdentityKey,
		},
		Uploader: client.Uploader{
			ServerURL: cfg.ServerURL,
			APIKey:    cfg.APIKey,
		},
	}
	err = runner.RunForever(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func writeExampleConfigIfMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	body := []byte("server_url: http://localhost:8080\napi_key: change-me\nidentity_key: change-me\nscan_interval: 5m\ncodex_home: \"\"\n")
	return os.WriteFile(path, body, 0o600)
}

func statePathForConfig(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "state.json")
}

func queuePathForConfig(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "pending.json")
}

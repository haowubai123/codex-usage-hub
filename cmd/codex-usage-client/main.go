package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
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
	case "install-service":
		return installServiceCommand(args[1:])
	case "uninstall-service":
		return uninstallServiceCommand(args[1:])
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

const (
	serviceName  = "codex-usage-client"
	launchdLabel = "com.codex-usage-client"
)

func installServiceCommand(args []string) error {
	fs := flag.NewFlagSet("install-service", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("usage: codex-usage-client install-service --config <path>")
	}

	absConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		return err
	}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		plistPath, err := launchdPlistPath()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
			return err
		}
		plist := client.LaunchdPlist(exePath, absConfigPath, launchdLabel)
		if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
			return err
		}
		return runExternal("launchctl", "load", plistPath)
	case "windows":
		return runExternal("sc.exe", client.WindowsCreateServiceArgs(exePath, absConfigPath, serviceName)...)
	default:
		return fmt.Errorf("install-service is unsupported on %s", runtime.GOOS)
	}
}

func uninstallServiceCommand(args []string) error {
	fs := flag.NewFlagSet("uninstall-service", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	// --config is accepted for command symmetry. macOS uses a fixed LaunchAgents plist path.
	configPath := fs.String("config", "", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = *configPath

	switch runtime.GOOS {
	case "darwin":
		plistPath, err := launchdPlistPath()
		if err != nil {
			return err
		}
		if err := runExternal("launchctl", "unload", plistPath); err != nil {
			return err
		}
		return os.Remove(plistPath)
	case "windows":
		return runExternal("sc.exe", "delete", serviceName)
	default:
		return fmt.Errorf("uninstall-service is unsupported on %s", runtime.GOOS)
	}
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

func launchdPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func runExternal(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, message)
}

package client

import (
	"strings"
	"testing"
)

func TestServiceLaunchdPlistContainsRunConfig(t *testing.T) {
	got := LaunchdPlist("/usr/local/bin/codex-usage-client", "/Users/a/client.yaml", "com.codex-usage-client")

	for _, want := range []string{
		"/usr/local/bin/codex-usage-client",
		"/Users/a/client.yaml",
		"run",
		"--config",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("LaunchdPlist() missing %q in:\n%s", want, got)
		}
	}
}

func TestServiceWindowsCreateServiceArgsContainsRunConfig(t *testing.T) {
	got := WindowsCreateServiceArgs(`C:\Program Files\Codex Usage\codex-usage-client.exe`, `C:\Users\a\client.yaml`, "codex-usage-client")
	joined := strings.Join(got, " ")

	for _, want := range []string{
		"codex-usage-client",
		`C:\Program Files\Codex Usage\codex-usage-client.exe`,
		`C:\Users\a\client.yaml`,
		"run",
		"--config",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("WindowsCreateServiceArgs() missing %q in %#v", want, got)
		}
	}
}

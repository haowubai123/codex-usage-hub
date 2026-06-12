# Codex Usage Tracker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go-based, no-UI macOS/Windows Codex usage client plus public SQLite-backed server and embedded web dashboard.

**Architecture:** The repository is a Go monorepo with two binaries: `codex-usage-client` and `codex-usage-server`. The client scans local Codex JSONL logs and uploads token events; the server stores events idempotently, calculates cost from server-side prices, maintains rollups, and serves APIs plus a static dashboard.

**Tech Stack:** Go 1.22+, standard library HTTP, SQLite via `modernc.org/sqlite`, YAML via `gopkg.in/yaml.v3`, embedded web assets via `embed`.

---

## File Structure

- Create `go.mod`: Go module and dependencies.
- Create `cmd/codex-usage-client/main.go`: client CLI entry point.
- Create `cmd/codex-usage-server/main.go`: server CLI entry point.
- Create `internal/shared/types.go`: shared event and API DTOs.
- Create `internal/shared/model.go`: model normalization helpers.
- Create `internal/client/config.go`: client config loading and default paths.
- Create `internal/client/state.go`: device id, scan cursor, and pending queue persistence.
- Create `internal/client/scanner.go`: Codex JSONL scanner.
- Create `internal/client/uploader.go`: HTTP ingest client.
- Create `internal/client/service.go`: Windows Service and macOS launchd install helpers.
- Create `internal/client/runner.go`: scan/upload loop.
- Create `internal/server/config.go`: server config loading.
- Create `internal/server/db.go`: SQLite open, schema migration, and seed data.
- Create `internal/server/cost.go`: cost calculation and pricing lookup.
- Create `internal/server/ingest.go`: authenticated ingest handler.
- Create `internal/server/query.go`: summary, breakdown, and events handlers.
- Create `internal/server/rollup.go`: hourly and daily rollup updates.
- Create `internal/server/http.go`: router and health handler.
- Create `web/index.html`: first dashboard UI.
- Create focused tests next to implementation files.

## Task 1: Go Module And Shared Types

**Files:**
- Create: `go.mod`
- Create: `internal/shared/types.go`
- Create: `internal/shared/model.go`
- Create: `internal/shared/model_test.go`
- Create: `cmd/codex-usage-client/main.go`
- Create: `cmd/codex-usage-server/main.go`

- [ ] **Step 1: Create module file**

Create `go.mod`:

```go
module codex-usage-tracker

go 1.22

require (
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.33.1
)
```

- [ ] **Step 2: Create shared DTOs**

Create `internal/shared/types.go` with exported request/response types:

```go
package shared

import "time"

type UsageEvent struct {
	EventID             string    `json:"event_id"`
	DeviceID            string    `json:"device_id"`
	IdentityKey         string    `json:"identity_key"`
	SessionID           string    `json:"session_id,omitempty"`
	Source              string    `json:"source"`
	Model               string    `json:"model"`
	InputTokens         int64     `json:"input_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	OccurredAt          time.Time `json:"occurred_at"`
}

type IngestRequest struct {
	DeviceID    string       `json:"device_id"`
	IdentityKey string       `json:"identity_key"`
	Platform    string       `json:"platform"`
	Events      []UsageEvent `json:"events"`
}

type IngestResponse struct {
	Accepted           int      `json:"accepted"`
	Duplicates         int      `json:"duplicates"`
	MissingPriceModels []string `json:"missing_price_models"`
}

type TimeBucket struct {
	BucketStart time.Time `json:"bucket_start"`
	InputTokens int64     `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CacheReadTokens int64 `json:"cache_read_tokens"`
	TotalTokens int64     `json:"total_tokens"`
	CostUSD *float64      `json:"cost_usd"`
	EventCount int64      `json:"event_count"`
}
```

- [ ] **Step 3: Add model normalization test**

Create `internal/shared/model_test.go`:

```go
package shared

import "testing"

func TestNormalizeModel(t *testing.T) {
	tests := map[string]string{
		"OpenAI/GPT-5.5-2026-05-14": "gpt-5.5",
		"gpt-5.4-20260514": "gpt-5.4",
		"gpt-5.2-codex@low": "gpt-5.2-codex-low",
		"glm-5.1": "glm-5.1",
	}
	for input, want := range tests {
		if got := NormalizeModel(input); got != want {
			t.Fatalf("NormalizeModel(%q) = %q, want %q", input, got, want)
		}
	}
}
```

- [ ] **Step 4: Implement model normalization**

Create `internal/shared/model.go`:

```go
package shared

import (
	"regexp"
	"strings"
)

var isoDateSuffix = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)
var compactDateSuffix = regexp.MustCompile(`-\d{8}$`)

func NormalizeModel(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ReplaceAll(name, "@", "-")
	name = isoDateSuffix.ReplaceAllString(name, "")
	name = compactDateSuffix.ReplaceAllString(name, "")
	return name
}
```

- [ ] **Step 5: Add binary stubs**

Create `cmd/codex-usage-client/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: codex-usage-client <init|run|install-service|uninstall-service>")
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "command %q is not implemented yet\n", os.Args[1])
	os.Exit(2)
}
```

Create `cmd/codex-usage-server/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: codex-usage-server serve --config server.yaml")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "serve is not implemented yet")
	os.Exit(2)
}
```

- [ ] **Step 6: Run test**

Run: `go test ./internal/shared`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod cmd internal/shared
git commit -m "chore: scaffold go module"
```

## Task 2: Client Config And State

**Files:**
- Create: `internal/client/config.go`
- Create: `internal/client/config_test.go`
- Create: `internal/client/state.go`
- Create: `internal/client/state_test.go`

- [ ] **Step 1: Write config tests**

Create tests covering default config location, `CODEX_HOME` fallback, explicit `codex_home`, and `scan_interval` parsing:

```go
func TestResolveCodexHomePrefersConfig(t *testing.T) {
	got := ResolveCodexHome(ClientConfig{CodexHome: `D:\codex-home`}, map[string]string{"CODEX_HOME": `D:\env-home`}, `C:\Users\a`)
	if got != `D:\codex-home` {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCodexHomeUsesEnv(t *testing.T) {
	got := ResolveCodexHome(ClientConfig{}, map[string]string{"CODEX_HOME": `/tmp/codex`}, `/home/a`)
	if got != `/tmp/codex` {
		t.Fatalf("got %q", got)
	}
}

func TestResolveCodexHomeUsesDefault(t *testing.T) {
	got := ResolveCodexHome(ClientConfig{}, map[string]string{}, `/home/a`)
	if got != `/home/a/.codex` {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Implement config loading**

`internal/client/config.go` must define:

```go
type ClientConfig struct {
	ServerURL    string        `yaml:"server_url"`
	APIKey       string        `yaml:"api_key"`
	IdentityKey  string        `yaml:"identity_key"`
	ScanInterval time.Duration `yaml:"-"`
	RawInterval  string        `yaml:"scan_interval"`
	CodexHome    string        `yaml:"codex_home"`
}
```

Functions:

- `DefaultConfigPath(goos string, home string, appData string) string`
- `LoadConfig(path string) (ClientConfig, error)`
- `ResolveCodexHome(cfg ClientConfig, env map[string]string, home string) string`
- `ValidateConfig(cfg ClientConfig) error`

- [ ] **Step 3: Write state tests**

Test that `LoadOrCreateDeviceID` creates a UUID-like string and returns the same value on the second call. Test that pending queue persists and drains events in FIFO order.

- [ ] **Step 4: Implement state persistence**

`internal/client/state.go` must define:

```go
type FileCursor struct {
	Path string `json:"path"`
	FileIdentity string `json:"file_identity"`
	Size int64 `json:"size"`
	ModTimeUnixNano int64 `json:"mod_time_unix_nano"`
	LineOffset int64 `json:"line_offset"`
	SessionID string `json:"session_id"`
	Model string `json:"model"`
	EventIndex int64 `json:"event_index"`
	PrevInput int64 `json:"prev_input"`
	PrevCacheRead int64 `json:"prev_cache_read"`
	PrevOutput int64 `json:"prev_output"`
}

type ScanState struct {
	DeviceID string `json:"device_id"`
	Files map[string]FileCursor `json:"files"`
}
```

Use atomic write by writing to `*.tmp` and renaming.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/client`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/client
git commit -m "feat: add client config and state"
```

## Task 3: Codex JSONL Scanner

**Files:**
- Create: `internal/client/scanner.go`
- Create: `internal/client/scanner_test.go`
- Create: `internal/client/testdata/codex-session.jsonl`

- [ ] **Step 1: Add scanner fixture**

Create `internal/client/testdata/codex-session.jsonl` with lines for `session_meta`, `turn_context`, a first cumulative `token_count`, a repeated all-zero boundary, and a second cumulative `token_count`.

- [ ] **Step 2: Write scanner tests**

Tests must assert:

- Two usage events are emitted from the fixture.
- Delta one equals the first cumulative usage.
- Delta two equals cumulative two minus cumulative one.
- The all-zero boundary event is skipped.
- Re-scanning with saved state emits zero events.
- `identity_key` from config is copied into every event.

- [ ] **Step 3: Implement scanner**

`internal/client/scanner.go` must expose:

```go
type Scanner struct {
	CodexHome string
	DeviceID string
	IdentityKey string
	Now func() time.Time
}

func (s Scanner) Scan(state *ScanState) ([]shared.UsageEvent, error)
```

Implementation rules:

- Collect files from `sessions/**/*.jsonl` and `archived_sessions/*.jsonl`.
- Process each file line by line.
- Parse only relevant JSON records.
- Preserve session id, model, event index, previous cumulative token values, and line offset in `ScanState`.
- Generate event id with SHA-256.

- [ ] **Step 4: Run scanner tests**

Run: `go test ./internal/client -run Scanner -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/client
git commit -m "feat: scan codex session usage"
```

## Task 4: Client Queue, Upload, And Run Loop

**Files:**
- Create: `internal/client/uploader.go`
- Create: `internal/client/uploader_test.go`
- Create: `internal/client/runner.go`
- Create: `internal/client/runner_test.go`
- Modify: `cmd/codex-usage-client/main.go`

- [ ] **Step 1: Write uploader tests**

Use `httptest.Server` to assert:

- `Authorization: Bearer <api_key>` is sent.
- Batches serialize as `shared.IngestRequest`.
- 401 returns a typed auth error.
- 500 keeps events retryable.

- [ ] **Step 2: Implement uploader**

Expose:

```go
type Uploader struct {
	ServerURL string
	APIKey string
	Client *http.Client
}

func (u Uploader) Upload(ctx context.Context, req shared.IngestRequest) (shared.IngestResponse, error)
```

- [ ] **Step 3: Write runner tests**

Use fake scanner and fake uploader interfaces to assert:

- Pending events are uploaded before newly scanned events.
- Upload failure leaves events in queue.
- Successful upload clears accepted events.

- [ ] **Step 4: Implement runner**

Expose:

```go
type Runner struct {
	Config ClientConfig
	StatePath string
	QueuePath string
	Scanner Scanner
	Uploader Uploader
}

func (r Runner) RunOnce(ctx context.Context) error
func (r Runner) RunForever(ctx context.Context) error
```

`RunForever` sleeps for `Config.ScanInterval` between cycles and stops on context cancellation.

- [ ] **Step 5: Wire client CLI**

`cmd/codex-usage-client/main.go` must support:

- `init --config <path>`
- `run --config <path>`

`init` writes an example config if the file does not exist and creates device state.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/client ./cmd/codex-usage-client`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/codex-usage-client internal/client
git commit -m "feat: upload client usage batches"
```

## Task 5: Client Service Install Commands

**Files:**
- Create: `internal/client/service.go`
- Create: `internal/client/service_test.go`
- Modify: `cmd/codex-usage-client/main.go`

- [ ] **Step 1: Write service artifact tests**

Test that macOS plist generation includes executable path, config path, and `run --config`. Test that Windows service command construction contains service name, executable path, and config path.

- [ ] **Step 2: Implement service helpers**

Expose:

```go
func LaunchdPlist(exePath string, configPath string, label string) string
func WindowsCreateServiceArgs(exePath string, configPath string, serviceName string) []string
```

`install-service`:

- On macOS, write plist to `~/Library/LaunchAgents/com.codex-usage-client.plist` and run `launchctl load`.
- On Windows, call `sc.exe create` with the binary path and config argument.

`uninstall-service`:

- On macOS, run `launchctl unload` and remove plist.
- On Windows, call `sc.exe delete`.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/client -run Service -v`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/codex-usage-client internal/client/service.go internal/client/service_test.go
git commit -m "feat: add client service installation"
```

## Task 6: Server Config And SQLite Schema

**Files:**
- Create: `internal/server/config.go`
- Create: `internal/server/config_test.go`
- Create: `internal/server/db.go`
- Create: `internal/server/db_test.go`
- Modify: `cmd/codex-usage-server/main.go`

- [ ] **Step 1: Write config tests**

Assert YAML config loads `listen_addr`, `public_base_url`, `api_key`, and `sqlite_path`, and validation rejects missing API key or SQLite path.

- [ ] **Step 2: Implement server config**

Define:

```go
type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	PublicBaseURL string `yaml:"public_base_url"`
	APIKey string `yaml:"api_key"`
	SQLitePath string `yaml:"sqlite_path"`
}
```

- [ ] **Step 3: Write DB schema tests**

Open a temp SQLite database and assert `devices`, `usage_events`, `model_prices`, and `usage_rollups` exist. Assert WAL mode is enabled.

- [ ] **Step 4: Implement DB open and migrations**

`internal/server/db.go` must expose:

```go
func OpenDB(path string) (*sql.DB, error)
func Migrate(ctx context.Context, db *sql.DB) error
func SeedModelPrices(ctx context.Context, db *sql.DB) error
```

Seed at least `gpt-5`, `gpt-5.5`, `gpt-5.4`, and `gpt-5.2-codex-low` with editable built-in prices.

- [ ] **Step 5: Wire server CLI**

`codex-usage-server serve --config server.yaml` loads config, opens DB, migrates, seeds prices, and starts HTTP server after Task 8 adds router.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/server -run 'Config|DB' -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/codex-usage-server internal/server
git commit -m "feat: add server config and sqlite schema"
```

## Task 7: Server Cost, Ingest, And Rollups

**Files:**
- Create: `internal/server/cost.go`
- Create: `internal/server/cost_test.go`
- Create: `internal/server/ingest.go`
- Create: `internal/server/ingest_test.go`
- Create: `internal/server/rollup.go`
- Create: `internal/server/rollup_test.go`

- [ ] **Step 1: Write cost tests**

Test Codex cache-inclusive input:

```go
usage := shared.UsageEvent{InputTokens: 1000, CacheReadTokens: 200, OutputTokens: 500}
price := ModelPrice{InputPerMillion: 3, CacheReadPerMillion: 0.3, OutputPerMillion: 15}
want := 0.0024 + 0.00006 + 0.0075
```

- [ ] **Step 2: Implement cost calculation**

Expose:

```go
type ModelPrice struct {
	Model string
	InputPerMillion float64
	CacheReadPerMillion float64
	CacheCreationPerMillion float64
	OutputPerMillion float64
}

func CalculateCost(event shared.UsageEvent, price ModelPrice) float64
```

- [ ] **Step 3: Write ingest tests**

Assert:

- Missing bearer token returns 401.
- Wrong bearer token returns 403.
- Valid event inserts once.
- Repeating the same event returns duplicate count.
- Missing price stores `pricing_status = missing` and null cost.
- Device `last_seen_at` updates.

- [ ] **Step 4: Implement ingest handler**

Expose:

```go
type Server struct {
	DB *sql.DB
	APIKey string
}

func (s Server) IngestHandler(w http.ResponseWriter, r *http.Request)
```

Validation:

- Event id, device id, identity key, source, model, and occurred_at are required.
- Token counts must be non-negative.
- Batch size is capped at 1000 events.

- [ ] **Step 5: Write rollup tests**

Insert two events in the same hour and one event on the next day. Assert hour/day rollups sum tokens, cost, and event count correctly.

- [ ] **Step 6: Implement rollups**

Expose:

```go
func UpdateRollups(ctx context.Context, tx *sql.Tx, event shared.UsageEvent, cost *float64) error
```

Use `INSERT ... ON CONFLICT DO UPDATE` for both hour and day buckets.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/server -run 'Cost|Ingest|Rollup' -v`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/server
git commit -m "feat: ingest usage events"
```

## Task 8: Server Query APIs And Dashboard

**Files:**
- Create: `internal/server/query.go`
- Create: `internal/server/query_test.go`
- Create: `internal/server/http.go`
- Create: `internal/server/http_test.go`
- Create: `web/index.html`
- Modify: `cmd/codex-usage-server/main.go`

- [ ] **Step 1: Write query tests**

Seed rollups and events, then assert:

- `/api/v1/summary` returns filtered buckets and totals.
- `/api/v1/breakdown?group_by=identity` groups by identity key.
- `/api/v1/breakdown?group_by=device` groups by device id.
- `/api/v1/breakdown?group_by=model` groups by model.
- `/api/v1/events?limit=2` returns newest two events.

- [ ] **Step 2: Implement query handlers**

Expose:

```go
func (s Server) SummaryHandler(w http.ResponseWriter, r *http.Request)
func (s Server) BreakdownHandler(w http.ResponseWriter, r *http.Request)
func (s Server) EventsHandler(w http.ResponseWriter, r *http.Request)
func (s Server) HealthHandler(w http.ResponseWriter, r *http.Request)
```

- [ ] **Step 3: Implement router**

`internal/server/http.go` must expose:

```go
func (s Server) Handler() http.Handler
```

Routes:

- `POST /api/v1/ingest`
- `GET /api/v1/summary`
- `GET /api/v1/breakdown`
- `GET /api/v1/events`
- `GET /healthz`
- `GET /`

- [ ] **Step 4: Create dashboard page**

Create `web/index.html` as a static operational dashboard. It should fetch summary and breakdown APIs, show cards for total tokens/cost/event count/missing prices, render a simple table-first view, and avoid requiring a frontend build tool.

- [ ] **Step 5: Wire server serve command**

`codex-usage-server serve --config server.yaml` must start the HTTP server with graceful shutdown on Ctrl+C.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/server ./cmd/codex-usage-server`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/codex-usage-server internal/server web
git commit -m "feat: serve usage dashboard"
```

## Task 9: End-To-End Fixtures And Documentation

**Files:**
- Create: `tests/e2e/e2e_test.go`
- Create: `tests/fixtures/device-a-1.jsonl`
- Create: `tests/fixtures/device-b-1.jsonl`
- Create: `tests/fixtures/device-a-2.jsonl`
- Create: `README.md`
- Create: `examples/client.config.yaml`
- Create: `examples/server.yaml`

- [ ] **Step 1: Create E2E fixtures**

Create three JSONL fixtures:

- Device 1 uses `identity_key = a`.
- Device 2 uses `identity_key = b`.
- Device 3 uses `identity_key = a`.

Each fixture must include at least one `session_meta`, one `turn_context`, and two `token_count` records.

- [ ] **Step 2: Write E2E test**

`tests/e2e/e2e_test.go` should:

- Start a temp server DB with `httptest.Server`.
- Run three scanner instances against fixture directories.
- Upload events through the real uploader.
- Query summary/breakdown.
- Assert identity `a` combines devices 1 and 3, while `b` remains separate.

- [ ] **Step 3: Write README**

Include:

- What the tool does.
- How identity merging works.
- How to run the server.
- How to initialize and run the client.
- How to install/uninstall the client service.
- How to add or edit model prices.

- [ ] **Step 4: Add example configs**

Create `examples/client.config.yaml` and `examples/server.yaml` using the confirmed fields.

- [ ] **Step 5: Run full verification**

Run:

```bash
go test ./...
go run ./cmd/codex-usage-client init --config ./tmp-client.yaml
go run ./cmd/codex-usage-server serve --config ./examples/server.yaml
```

Expected:

- `go test ./...` passes.
- Client init writes config/state.
- Server starts and serves `/healthz`.

- [ ] **Step 6: Commit**

```bash
git add README.md examples tests
git commit -m "test: add e2e coverage and docs"
```

## Plan Self-Review

- Spec coverage: Covers local Codex log scanning, macOS/Windows no-UI client, public server, Go implementation, SQLite, config-file identity, service install commands, web dashboard, and tests.
- Red-flag scan: No task relies on an unspecified future decision.
- Type consistency: Shared `UsageEvent`, `IngestRequest`, and `IngestResponse` names are used consistently across client and server tasks.
- Scope check: This plan is a single first-version implementation. Proxy capture, PostgreSQL, multi-tenant auth, and automatic pricing refresh remain out of scope.

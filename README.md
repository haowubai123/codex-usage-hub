# Codex Usage Tracker

Codex Usage Tracker records token usage from local Codex session logs and uploads it to a small HTTP server backed by SQLite. The server stores raw usage events, maintains hourly and daily rollups, applies model pricing when a price row is available, and exposes JSON APIs plus a web dashboard.

## Identity Merging

Each client has a generated `device_id` stored in its local state file. Each client config also has an `identity_key`, such as a person, team member, or billing identity.

The server stores both values. Device-level views stay separate by `device_id`, while identity-level summaries merge every device that reports the same `identity_key`. For example, a laptop and desktop configured with `identity_key: alice` are combined in `/api/v1/breakdown?group_by=identity`, while `/api/v1/breakdown?group_by=device` still shows the two devices separately.

## Run The Server

Create a server config:

```yaml
listen_addr: ":8080"
public_base_url: http://localhost:8080
api_key: change-me
sqlite_path: ./data/codex-usage.sqlite
```

Start the server:

```bash
go run ./cmd/codex-usage-server serve --config ./examples/server.yaml
```

The server creates the SQLite database path if needed, runs migrations, seeds built-in model prices, and listens on `listen_addr`. Check it with:

```bash
curl http://localhost:8080/healthz
```

## Initialize And Run The Client

Create a client config and state file:

```bash
go run ./cmd/codex-usage-client init --config ./client.config.yaml
```

If the config file does not exist, `init` writes a starter config:

```yaml
server_url: http://localhost:8080
api_key: change-me
identity_key: change-me
scan_interval: 5m
codex_home: ""
```

Edit `api_key` to match the server and set `identity_key` to the identity you want merged across devices. Leave `codex_home` empty to use `CODEX_HOME` or the default `~/.codex`; set it explicitly to scan another Codex data directory.

Run one long-lived client process:

```bash
go run ./cmd/codex-usage-client run --config ./client.config.yaml
```

The client scans `sessions/**/*.jsonl` and `archived_sessions/*.jsonl`, uploads new token usage events to `/api/v1/ingest`, and keeps local cursor and pending-upload state next to the config file.

## Install Or Uninstall The Client Service

Build or install the client binary first, then install the service with the same config path you tested manually:

```bash
go run ./cmd/codex-usage-client install-service --config ./client.config.yaml
```

On macOS this writes and loads a LaunchAgent named `com.codex-usage-client`. On Windows it creates a service named `codex-usage-client` with `sc.exe`. Linux service installation is not currently implemented.

Uninstall:

```bash
go run ./cmd/codex-usage-client uninstall-service --config ./client.config.yaml
```

On macOS the command unloads and removes the LaunchAgent. On Windows it deletes the Windows service.

## Model Prices

Model prices live in the SQLite `model_prices` table:

```sql
CREATE TABLE model_prices (
  model TEXT PRIMARY KEY,
  input_per_million REAL NOT NULL,
  cache_read_per_million REAL NOT NULL DEFAULT 0,
  cache_creation_per_million REAL NOT NULL DEFAULT 0,
  output_per_million REAL NOT NULL,
  updated_at TEXT NOT NULL
);
```

The server seeds built-in rows with `INSERT OR IGNORE`, so your edits are preserved. Add or update prices with SQLite:

```bash
sqlite3 ./data/codex-usage.sqlite \
  "INSERT INTO model_prices (model, input_per_million, cache_read_per_million, cache_creation_per_million, output_per_million, updated_at)
   VALUES ('gpt-5.2-codex-low', 1, 0.1, 1, 3, 'manual')
   ON CONFLICT(model) DO UPDATE SET
     input_per_million = excluded.input_per_million,
     cache_read_per_million = excluded.cache_read_per_million,
     cache_creation_per_million = excluded.cache_creation_per_million,
     output_per_million = excluded.output_per_million,
     updated_at = excluded.updated_at;"
```

Scanner model names are normalized before pricing: names are lowercased, provider prefixes are removed, `@` becomes `-`, and date suffixes like `-2026-05-14` are stripped. Use the normalized model name in `model_prices`.

## HTTP APIs

Useful read endpoints:

```text
GET /api/v1/summary?bucket=day
GET /api/v1/breakdown?group_by=identity
GET /api/v1/breakdown?group_by=device
GET /api/v1/events?limit=100
```

Optional filters include `identity_key`, `device_id`, `model`, `from`, and `to`.

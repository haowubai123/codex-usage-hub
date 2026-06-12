package client

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"codex-usage-tracker/internal/shared"
)

type Scanner struct {
	CodexHome   string
	DeviceID    string
	IdentityKey string
	Now         func() time.Time
}

func (s Scanner) Scan(state *ScanState) ([]shared.UsageEvent, error) {
	if state == nil {
		return nil, errors.New("scan state is nil")
	}
	if state.Files == nil {
		state.Files = map[string]FileCursor{}
	}

	paths, err := s.sessionFiles()
	if err != nil {
		return nil, err
	}

	now := s.now()
	var events []shared.UsageEvent
	for _, path := range paths {
		fileEvents, err := s.scanFile(path, now, state)
		if err != nil {
			return nil, err
		}
		events = append(events, fileEvents...)
	}
	return events, nil
}

func (s Scanner) sessionFiles() ([]string, error) {
	var paths []string

	sessionsDir := filepath.Join(s.CodexHome, "sessions")
	err := filepath.WalkDir(sessionsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		paths = append(paths, filepath.Clean(path))
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	archivedDir := filepath.Join(s.CodexHome, "archived_sessions")
	archived, err := filepath.Glob(filepath.Join(archivedDir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, path := range archived {
		paths = append(paths, filepath.Clean(path))
	}

	sort.Strings(paths)
	return paths, nil
}

func (s Scanner) scanFile(path string, scanTime time.Time, state *ScanState) ([]shared.UsageEvent, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	fileIdentity := fileIdentity(path)
	cursor := state.Files[path]
	if cursor.Path == "" {
		cursor.Path = path
	}
	if cursor.FileIdentity == "" {
		cursor.FileIdentity = fileIdentity
	}
	if info.Size() < cursor.Size {
		cursor = FileCursor{Path: path, FileIdentity: fileIdentity}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	lineScanner := bufio.NewScanner(file)
	lineScanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var events []shared.UsageEvent
	var line int64
	for lineScanner.Scan() {
		line++
		if line <= cursor.LineOffset {
			continue
		}

		text := strings.TrimSpace(lineScanner.Text())
		if text == "" {
			continue
		}

		record, err := parseScannerRecord([]byte(text))
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}

		switch record.Type {
		case "session_meta":
			if sessionID := scannerString(record.Payload, "session_id", "sessionId", "id"); sessionID != "" {
				cursor.SessionID = sessionID
			}
		case "turn_context":
			model := scannerString(record.Payload, "model")
			if model == "" {
				infoPayload := scannerObject(record.Payload, "info")
				model = scannerString(infoPayload, "model")
			}
			if model != "" {
				cursor.Model = shared.NormalizeModel(model)
			}
		case "event_msg":
			event, emitted, usage, err := s.usageEventFromRecord(record, cursor, path, scanTime)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, line, err)
			}
			if emitted {
				events = append(events, event)
				cursor.EventIndex++
			}
			if usage != nil && !usage.Incremental && !usage.AllZero() {
				cursor.PrevInput = usage.InputTokens
				cursor.PrevCacheRead = usage.CacheReadTokens
				cursor.PrevOutput = usage.OutputTokens
			}
			if usage != nil && usage.Incremental && emitted {
				cursor.PrevInput += usage.InputTokens
				cursor.PrevCacheRead += usage.CacheReadTokens
				cursor.PrevOutput += usage.OutputTokens
			}
		}
	}
	if err := lineScanner.Err(); err != nil {
		return nil, err
	}

	cursor.Path = path
	cursor.FileIdentity = fileIdentity
	cursor.Size = info.Size()
	cursor.ModTimeUnixNano = info.ModTime().UnixNano()
	cursor.LineOffset = line
	state.Files[path] = cursor

	return events, nil
}

func (s Scanner) usageEventFromRecord(record scannerRecord, cursor FileCursor, path string, fallback time.Time) (shared.UsageEvent, bool, *scannerTokenUsage, error) {
	if scannerString(record.Payload, "type") != "token_count" {
		return shared.UsageEvent{}, false, nil, nil
	}

	usage, err := extractTokenUsage(record.Payload)
	if err != nil {
		return shared.UsageEvent{}, false, nil, err
	}
	if usage == nil {
		return shared.UsageEvent{}, false, nil, nil
	}

	input := usage.InputTokens
	cacheRead := usage.CacheReadTokens
	output := usage.OutputTokens
	if !usage.Incremental {
		if usage.AllZero() {
			return shared.UsageEvent{}, false, usage, nil
		}
		input -= cursor.PrevInput
		cacheRead -= cursor.PrevCacheRead
		output -= cursor.PrevOutput
	}
	if input < 0 || cacheRead < 0 || output < 0 {
		return shared.UsageEvent{}, false, usage, nil
	}
	if cacheRead > input {
		cacheRead = input
	}
	if input == 0 && cacheRead == 0 && output == 0 {
		return shared.UsageEvent{}, false, usage, nil
	}

	occurredAt := record.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = fallback
	}
	eventIndex := cursor.EventIndex + 1
	event := shared.UsageEvent{
		EventID:         scannerEventID(s.DeviceID, cursor.FileIdentity, path, cursor.SessionID, eventIndex, occurredAt),
		DeviceID:        s.DeviceID,
		IdentityKey:     s.IdentityKey,
		SessionID:       cursor.SessionID,
		Source:          "codex_session",
		Model:           cursor.Model,
		InputTokens:     input,
		CacheReadTokens: cacheRead,
		OutputTokens:    output,
		OccurredAt:      occurredAt.UTC(),
	}
	return event, true, usage, nil
}

func (s Scanner) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type scannerRecord struct {
	Type       string
	Payload    map[string]json.RawMessage
	OccurredAt time.Time
}

type scannerTokenUsage struct {
	InputTokens     int64
	CacheReadTokens int64
	OutputTokens    int64
	Incremental     bool
}

func (u scannerTokenUsage) AllZero() bool {
	return u.InputTokens == 0 && u.CacheReadTokens == 0 && u.OutputTokens == 0
}

func parseScannerRecord(data []byte) (scannerRecord, error) {
	var raw struct {
		Type    string                     `json:"type"`
		Payload map[string]json.RawMessage `json:"payload"`
		Fields  map[string]json.RawMessage `json:"-"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return scannerRecord{}, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return scannerRecord{}, err
	}

	return scannerRecord{
		Type:       raw.Type,
		Payload:    raw.Payload,
		OccurredAt: scannerTime(fields, raw.Payload),
	}, nil
}

func extractTokenUsage(payload map[string]json.RawMessage) (*scannerTokenUsage, error) {
	infoPayload := scannerObject(payload, "info")
	if infoPayload == nil {
		return nil, nil
	}

	if total := scannerObject(infoPayload, "total_token_usage"); total != nil {
		usage, err := parseTokenUsage(total)
		if err != nil {
			return nil, err
		}
		return usage, nil
	}

	if last := scannerObject(infoPayload, "last_token_usage"); last != nil {
		usage, err := parseTokenUsage(last)
		if err != nil {
			return nil, err
		}
		usage.Incremental = true
		return usage, nil
	}

	return nil, nil
}

func parseTokenUsage(payload map[string]json.RawMessage) (*scannerTokenUsage, error) {
	input, err := scannerInt(payload, "input_tokens")
	if err != nil {
		return nil, err
	}
	cacheRead, err := scannerInt(payload, "cached_input_tokens", "cache_read_input_tokens")
	if err != nil {
		return nil, err
	}
	output, err := scannerInt(payload, "output_tokens")
	if err != nil {
		return nil, err
	}
	if cacheRead > input {
		cacheRead = input
	}
	return &scannerTokenUsage{
		InputTokens:     input,
		CacheReadTokens: cacheRead,
		OutputTokens:    output,
	}, nil
}

func scannerString(payload map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			return value
		}
	}
	return ""
}

func scannerObject(payload map[string]json.RawMessage, key string) map[string]json.RawMessage {
	raw, ok := payload[key]
	if !ok {
		return nil
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func scannerInt(payload map[string]json.RawMessage, keys ...string) (int64, error) {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var number json.Number
		if err := json.Unmarshal(raw, &number); err == nil {
			return number.Int64()
		}
		var value float64
		if err := json.Unmarshal(raw, &value); err == nil {
			return int64(value), nil
		}
		return 0, fmt.Errorf("token field %q is not numeric", key)
	}
	return 0, nil
}

func scannerTime(fields map[string]json.RawMessage, payload map[string]json.RawMessage) time.Time {
	for _, source := range []map[string]json.RawMessage{fields, payload} {
		if source == nil {
			continue
		}
		for _, key := range []string{"timestamp", "time", "created_at", "createdAt"} {
			raw, ok := source[key]
			if !ok {
				continue
			}
			if t := parseScannerTime(raw); !t.IsZero() {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}

func parseScannerTime(raw json.RawMessage) time.Time {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return t
		}
		if unix, err := strconv.ParseInt(text, 10, 64); err == nil {
			return unixTime(unix)
		}
	}

	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		if unix, err := number.Int64(); err == nil {
			return unixTime(unix)
		}
	}
	return time.Time{}
}

func unixTime(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func scannerEventID(deviceID string, fileIdentity string, path string, sessionID string, eventIndex int64, occurredAt time.Time) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00%d\x00%s", deviceID, fileIdentity, path, sessionID, eventIndex, occurredAt.UTC().Format(time.RFC3339Nano))
	return hex.EncodeToString(hash.Sum(nil))
}

func fileIdentity(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

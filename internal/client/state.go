package client

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"codex-usage-tracker/internal/shared"
)

type FileCursor struct {
	Path            string `json:"path"`
	FileIdentity    string `json:"file_identity"`
	Size            int64  `json:"size"`
	ModTimeUnixNano int64  `json:"mod_time_unix_nano"`
	LineOffset      int64  `json:"line_offset"`
	SessionID       string `json:"session_id"`
	Model           string `json:"model"`
	EventIndex      int64  `json:"event_index"`
	PrevInput       int64  `json:"prev_input"`
	PrevCacheRead   int64  `json:"prev_cache_read"`
	PrevOutput      int64  `json:"prev_output"`
}

type ScanState struct {
	DeviceID string                `json:"device_id"`
	Files    map[string]FileCursor `json:"files"`
}

func LoadState(path string) (ScanState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ScanState{Files: map[string]FileCursor{}}, nil
	}
	if err != nil {
		return ScanState{}, err
	}

	var state ScanState
	if err := json.Unmarshal(data, &state); err != nil {
		return ScanState{}, err
	}
	if state.Files == nil {
		state.Files = map[string]FileCursor{}
	}
	return state, nil
}

func SaveState(path string, state ScanState) error {
	if state.Files == nil {
		state.Files = map[string]FileCursor{}
	}
	return writeJSONAtomic(path, state)
}

func LoadOrCreateDeviceID(path string) (string, error) {
	state, err := LoadState(path)
	if err != nil {
		return "", err
	}
	if state.DeviceID != "" {
		return state.DeviceID, nil
	}

	deviceID, err := newUUIDLike()
	if err != nil {
		return "", err
	}
	state.DeviceID = deviceID
	if err := SaveState(path, state); err != nil {
		return "", err
	}
	return deviceID, nil
}

func EnqueuePending(path string, events []shared.UsageEvent) error {
	pending, err := LoadPending(path)
	if err != nil {
		return err
	}
	pending = append(pending, events...)
	return ReplacePending(path, pending)
}

func LoadPending(path string) ([]shared.UsageEvent, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []shared.UsageEvent{}, nil
	}
	if err != nil {
		return nil, err
	}

	var events []shared.UsageEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, err
	}
	if events == nil {
		return []shared.UsageEvent{}, nil
	}
	return events, nil
}

func ReplacePending(path string, events []shared.UsageEvent) error {
	if events == nil {
		events = []shared.UsageEvent{}
	}
	return writeJSONAtomic(path, events)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return fmt.Errorf("%w; close temp file: %v", err, closeErr)
		}
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func newUUIDLike() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	encoded := make([]byte, 32)
	hex.Encode(encoded, b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}

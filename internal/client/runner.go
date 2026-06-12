package client

import (
	"context"
	"errors"
	"runtime"
	"time"

	"codex-usage-tracker/internal/shared"
)

type usageScanner interface {
	Scan(state *ScanState) ([]shared.UsageEvent, error)
}

type usageUploader interface {
	Upload(ctx context.Context, req shared.IngestRequest) (shared.IngestResponse, error)
}

type Runner struct {
	Config    ClientConfig
	StatePath string
	QueuePath string
	Scanner   Scanner
	Uploader  Uploader

	// ScannerImpl optionally overrides Scanner in tests.
	ScannerImpl usageScanner
	// UploaderImpl optionally overrides Uploader in tests.
	UploaderImpl usageUploader
}

func (r Runner) RunOnce(ctx context.Context) error {
	state, err := LoadState(r.StatePath)
	if err != nil {
		return err
	}

	pending, err := LoadPending(r.QueuePath)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		remaining, err := r.uploadEvents(ctx, state.DeviceID, pending)
		if err != nil {
			return err
		}
		if err := ReplacePending(r.QueuePath, remaining); err != nil {
			return err
		}
		if len(remaining) > 0 {
			return nil
		}
	}

	events, err := r.scanner().Scan(&state)
	if err != nil {
		return err
	}
	if err := SaveState(r.StatePath, state); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	if err := ReplacePending(r.QueuePath, events); err != nil {
		return err
	}

	remaining, err := r.uploadEvents(ctx, state.DeviceID, events)
	if err != nil {
		return err
	}
	return ReplacePending(r.QueuePath, remaining)
}

func (r Runner) RunForever(ctx context.Context) error {
	interval := r.Config.ScanInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	for {
		if err := r.RunOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			var authErr *AuthError
			if errors.As(err, &authErr) {
				return err
			}
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r Runner) uploadEvents(ctx context.Context, deviceID string, events []shared.UsageEvent) ([]shared.UsageEvent, error) {
	req := shared.IngestRequest{
		DeviceID:    requestDeviceID(deviceID, events),
		IdentityKey: requestIdentityKey(r.Config.IdentityKey, events),
		Platform:    runtime.GOOS,
		Events:      events,
	}
	resp, err := r.uploader().Upload(ctx, req)
	if err != nil {
		return events, err
	}
	drop := resp.Accepted + resp.Duplicates
	if drop < 0 {
		drop = 0
	}
	if drop > len(events) {
		drop = len(events)
	}
	return append([]shared.UsageEvent(nil), events[drop:]...), nil
}

func (r Runner) scanner() usageScanner {
	if r.ScannerImpl != nil {
		return r.ScannerImpl
	}
	return r.Scanner
}

func (r Runner) uploader() usageUploader {
	if r.UploaderImpl != nil {
		return r.UploaderImpl
	}
	return r.Uploader
}

func requestDeviceID(deviceID string, events []shared.UsageEvent) string {
	if deviceID != "" {
		return deviceID
	}
	if len(events) > 0 {
		return events[0].DeviceID
	}
	return ""
}

func requestIdentityKey(identityKey string, events []shared.UsageEvent) string {
	if identityKey != "" {
		return identityKey
	}
	if len(events) > 0 {
		return events[0].IdentityKey
	}
	return ""
}

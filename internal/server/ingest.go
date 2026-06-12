package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"codex-usage-tracker/internal/shared"
)

const maxIngestEvents = 1000

type Server struct {
	DB     *sql.DB
	APIKey string
}

func (s Server) IngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorizeIngest(w, r) {
		return
	}

	var req shared.IngestRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}
	if len(req.Events) > maxIngestEvents {
		http.Error(w, "too many events", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "begin transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	resp, err := s.ingestEvents(ctx, tx, req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errInvalidIngestEvent) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "commit transaction", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}

func (s Server) authorizeIngest(w http.ResponseWriter, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) || strings.TrimSpace(strings.TrimPrefix(auth, prefix)) == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return false
	}
	if strings.TrimSpace(strings.TrimPrefix(auth, prefix)) != s.APIKey {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

var errInvalidIngestEvent = errors.New("invalid ingest event")

func (s Server) ingestEvents(ctx context.Context, tx *sql.Tx, req shared.IngestRequest) (shared.IngestResponse, error) {
	var resp shared.IngestResponse
	missingPrices := make(map[string]struct{})

	for i, event := range req.Events {
		normalizedModel, err := validateUsageEvent(event)
		if err != nil {
			return resp, fmt.Errorf("%w %d: %v", errInvalidIngestEvent, i, err)
		}
		event.Model = normalizedModel
		event.OccurredAt = event.OccurredAt.UTC()

		if err := upsertDevice(ctx, tx, req, event); err != nil {
			return resp, err
		}

		price, found, err := loadModelPrice(ctx, tx, normalizedModel)
		if err != nil {
			return resp, err
		}

		var cost *float64
		pricingStatus := "missing"
		if found {
			value := CalculateCost(event, price)
			cost = &value
			pricingStatus = "priced"
		} else {
			missingPrices[normalizedModel] = struct{}{}
		}

		inserted, err := insertUsageEvent(ctx, tx, event, pricingStatus, cost)
		if err != nil {
			return resp, err
		}
		if !inserted {
			resp.Duplicates++
			continue
		}

		if err := UpdateRollups(ctx, tx, event, cost); err != nil {
			return resp, err
		}
		resp.Accepted++
	}

	resp.MissingPriceModels = sortedKeys(missingPrices)
	return resp, nil
}

func validateUsageEvent(event shared.UsageEvent) (string, error) {
	if strings.TrimSpace(event.EventID) == "" {
		return "", errors.New("event_id is required")
	}
	if strings.TrimSpace(event.DeviceID) == "" {
		return "", errors.New("device_id is required")
	}
	if strings.TrimSpace(event.IdentityKey) == "" {
		return "", errors.New("identity_key is required")
	}
	if strings.TrimSpace(event.Source) == "" {
		return "", errors.New("source is required")
	}
	if strings.TrimSpace(event.Model) == "" {
		return "", errors.New("model is required")
	}
	if event.OccurredAt.IsZero() {
		return "", errors.New("occurred_at is required")
	}
	if event.InputTokens < 0 || event.CacheReadTokens < 0 || event.CacheCreationTokens < 0 || event.OutputTokens < 0 {
		return "", errors.New("token counts must be non-negative")
	}

	model := shared.NormalizeModel(event.Model)
	if model == "" {
		return "", errors.New("model is required")
	}
	return model, nil
}

func upsertDevice(ctx context.Context, tx *sql.Tx, req shared.IngestRequest, event shared.UsageEvent) error {
	seenAt := event.OccurredAt.UTC().Format(time.RFC3339Nano)
	platform := strings.TrimSpace(req.Platform)

	_, err := tx.ExecContext(ctx, `INSERT INTO devices (
		device_id, identity_key, platform, first_seen_at, last_seen_at
	) VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(device_id) DO UPDATE SET
		identity_key = excluded.identity_key,
		platform = CASE WHEN excluded.platform = '' THEN devices.platform ELSE excluded.platform END,
		first_seen_at = CASE WHEN excluded.first_seen_at < devices.first_seen_at THEN excluded.first_seen_at ELSE devices.first_seen_at END,
		last_seen_at = CASE WHEN excluded.last_seen_at > devices.last_seen_at THEN excluded.last_seen_at ELSE devices.last_seen_at END`,
		event.DeviceID,
		event.IdentityKey,
		platform,
		seenAt,
		seenAt,
	)
	return err
}

func loadModelPrice(ctx context.Context, tx *sql.Tx, model string) (ModelPrice, bool, error) {
	var price ModelPrice
	err := tx.QueryRowContext(ctx, `SELECT model, input_per_million, cache_read_per_million, cache_creation_per_million, output_per_million
		FROM model_prices
		WHERE model = ?`, model).
		Scan(&price.Model, &price.InputPerMillion, &price.CacheReadPerMillion, &price.CacheCreationPerMillion, &price.OutputPerMillion)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelPrice{}, false, nil
	}
	if err != nil {
		return ModelPrice{}, false, err
	}
	return price, true, nil
}

func insertUsageEvent(ctx context.Context, tx *sql.Tx, event shared.UsageEvent, pricingStatus string, cost *float64) (bool, error) {
	var costValue any
	if cost != nil {
		costValue = *cost
	}

	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO usage_events (
		event_id,
		device_id,
		identity_key,
		session_id,
		source,
		model,
		input_tokens,
		cache_read_tokens,
		cache_creation_tokens,
		output_tokens,
		occurred_at,
		received_at,
		pricing_status,
		cost_usd
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID,
		event.DeviceID,
		event.IdentityKey,
		event.SessionID,
		event.Source,
		event.Model,
		event.InputTokens,
		event.CacheReadTokens,
		event.CacheCreationTokens,
		event.OutputTokens,
		event.OccurredAt.UTC().Format(time.RFC3339Nano),
		time.Now().UTC().Format(time.RFC3339Nano),
		pricingStatus,
		costValue,
	)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

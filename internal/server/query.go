package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"codex-usage-tracker/internal/shared"
)

const (
	defaultEventLimit = 100
	maxEventLimit     = 500
)

type UsageTotals struct {
	InputTokens         int64    `json:"input_tokens"`
	CacheReadTokens     int64    `json:"cache_read_tokens"`
	CacheCreationTokens int64    `json:"cache_creation_tokens"`
	OutputTokens        int64    `json:"output_tokens"`
	TotalTokens         int64    `json:"total_tokens"`
	CostUSD             *float64 `json:"cost_usd"`
	EventCount          int64    `json:"event_count"`
	MissingPriceCount   int64    `json:"missing_price_count"`
}

type SummaryBucket struct {
	BucketStart time.Time `json:"bucket_start"`
	UsageTotals
}

type SummaryResponse struct {
	Bucket  string          `json:"bucket"`
	Buckets []SummaryBucket `json:"buckets"`
	Totals  UsageTotals     `json:"totals"`
}

type BreakdownItem struct {
	Key string `json:"key"`
	UsageTotals
}

type BreakdownResponse struct {
	GroupBy string          `json:"group_by"`
	Items   []BreakdownItem `json:"items"`
	Totals  UsageTotals     `json:"totals"`
}

type StoredUsageEvent struct {
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
	TotalTokens         int64     `json:"total_tokens"`
	OccurredAt          time.Time `json:"occurred_at"`
	ReceivedAt          time.Time `json:"received_at"`
	PricingStatus       string    `json:"pricing_status"`
	CostUSD             *float64  `json:"cost_usd"`
}

type EventsResponse struct {
	Events []StoredUsageEvent `json:"events"`
}

type queryFilters struct {
	IdentityKey string
	DeviceID    string
	Model       string
	From        *time.Time
	To          *time.Time
}

func (s Server) SummaryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "day"
	}
	if bucket != "hour" && bucket != "day" {
		http.Error(w, "bucket must be hour or day", http.StatusBadRequest)
		return
	}

	filters, err := parseQueryFilters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conditions, args := rollupConditions("r", filters)
	args = append([]any{bucket}, args...)
	rows, err := s.DB.QueryContext(r.Context(), `SELECT
			r.bucket_start,
			SUM(r.input_tokens),
			SUM(r.cache_read_tokens),
			SUM(r.cache_creation_tokens),
			SUM(r.output_tokens),
			SUM(r.total_tokens),
			SUM(r.cost_usd),
			SUM(r.event_count)
		FROM usage_rollups r
		WHERE r.bucket_granularity = ?`+conditions+`
		GROUP BY r.bucket_start
		ORDER BY r.bucket_start ASC`, args...)
	if err != nil {
		http.Error(w, "query summary", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var resp SummaryResponse
	resp.Bucket = bucket
	for rows.Next() {
		var item SummaryBucket
		var bucketStart string
		var cost sql.NullFloat64
		if err := rows.Scan(&bucketStart, &item.InputTokens, &item.CacheReadTokens, &item.CacheCreationTokens, &item.OutputTokens, &item.TotalTokens, &cost, &item.EventCount); err != nil {
			http.Error(w, "scan summary", http.StatusInternalServerError)
			return
		}
		item.BucketStart, err = time.Parse(time.RFC3339Nano, bucketStart)
		if err != nil {
			http.Error(w, "parse bucket time", http.StatusInternalServerError)
			return
		}
		item.CostUSD = nullableFloat(cost)
		resp.Buckets = append(resp.Buckets, item)
		addTotals(&resp.Totals, item.UsageTotals)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "iterate summary", http.StatusInternalServerError)
		return
	}

	resp.Totals.MissingPriceCount, err = s.countMissingPrices(r, filters)
	if err != nil {
		http.Error(w, "query missing prices", http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s Server) BreakdownHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groupBy := r.URL.Query().Get("group_by")
	if groupBy == "" {
		groupBy = "identity"
	}
	groupColumn, err := breakdownColumn(groupBy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters, err := parseQueryFilters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conditions, args := rollupConditions("r", filters)
	args = append([]any{"day"}, args...)
	rows, err := s.DB.QueryContext(r.Context(), `SELECT
			`+groupColumn+`,
			SUM(r.input_tokens),
			SUM(r.cache_read_tokens),
			SUM(r.cache_creation_tokens),
			SUM(r.output_tokens),
			SUM(r.total_tokens),
			SUM(r.cost_usd),
			SUM(r.event_count)
		FROM usage_rollups r
		WHERE r.bucket_granularity = ?`+conditions+`
		GROUP BY `+groupColumn+`
		ORDER BY SUM(r.total_tokens) DESC, `+groupColumn+` ASC`, args...)
	if err != nil {
		http.Error(w, "query breakdown", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	resp := BreakdownResponse{GroupBy: groupBy}
	for rows.Next() {
		var item BreakdownItem
		var cost sql.NullFloat64
		if err := rows.Scan(&item.Key, &item.InputTokens, &item.CacheReadTokens, &item.CacheCreationTokens, &item.OutputTokens, &item.TotalTokens, &cost, &item.EventCount); err != nil {
			http.Error(w, "scan breakdown", http.StatusInternalServerError)
			return
		}
		item.CostUSD = nullableFloat(cost)
		resp.Items = append(resp.Items, item)
		addTotals(&resp.Totals, item.UsageTotals)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "iterate breakdown", http.StatusInternalServerError)
		return
	}

	resp.Totals.MissingPriceCount, err = s.countMissingPrices(r, filters)
	if err != nil {
		http.Error(w, "query missing prices", http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s Server) EventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filters, err := parseQueryFilters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conditions, args := eventConditions("e", filters)
	args = append(args, limit)
	rows, err := s.DB.QueryContext(r.Context(), `SELECT
			e.event_id,
			e.device_id,
			e.identity_key,
			e.session_id,
			e.source,
			e.model,
			e.input_tokens,
			e.cache_read_tokens,
			e.cache_creation_tokens,
			e.output_tokens,
			e.occurred_at,
			e.received_at,
			e.pricing_status,
			e.cost_usd
		FROM usage_events e
		WHERE 1 = 1`+conditions+`
		ORDER BY e.occurred_at DESC, e.event_id DESC
		LIMIT ?`, args...)
	if err != nil {
		http.Error(w, "query events", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var resp EventsResponse
	for rows.Next() {
		var event StoredUsageEvent
		var occurredAt, receivedAt string
		var cost sql.NullFloat64
		if err := rows.Scan(&event.EventID, &event.DeviceID, &event.IdentityKey, &event.SessionID, &event.Source, &event.Model, &event.InputTokens, &event.CacheReadTokens, &event.CacheCreationTokens, &event.OutputTokens, &occurredAt, &receivedAt, &event.PricingStatus, &cost); err != nil {
			http.Error(w, "scan events", http.StatusInternalServerError)
			return
		}
		event.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			http.Error(w, "parse occurred_at", http.StatusInternalServerError)
			return
		}
		event.ReceivedAt, err = time.Parse(time.RFC3339Nano, receivedAt)
		if err != nil {
			http.Error(w, "parse received_at", http.StatusInternalServerError)
			return
		}
		event.TotalTokens = event.InputTokens + event.CacheReadTokens + event.CacheCreationTokens + event.OutputTokens
		event.CostUSD = nullableFloat(cost)
		resp.Events = append(resp.Events, event)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "iterate events", http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func parseQueryFilters(r *http.Request) (queryFilters, error) {
	values := r.URL.Query()
	var filters queryFilters
	filters.IdentityKey = strings.TrimSpace(values.Get("identity_key"))
	filters.DeviceID = strings.TrimSpace(values.Get("device_id"))
	filters.Model = strings.TrimSpace(values.Get("model"))
	if filters.Model != "" {
		filters.Model = shared.NormalizeModel(filters.Model)
	}

	if from := strings.TrimSpace(values.Get("from")); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339Nano, from)
		}
		if err != nil {
			return queryFilters{}, fmt.Errorf("from must be RFC3339")
		}
		utc := parsed.UTC()
		filters.From = &utc
	}
	if to := strings.TrimSpace(values.Get("to")); to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339Nano, to)
		}
		if err != nil {
			return queryFilters{}, fmt.Errorf("to must be RFC3339")
		}
		utc := parsed.UTC()
		filters.To = &utc
	}
	return filters, nil
}

func parseLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultEventLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	if limit > maxEventLimit {
		return maxEventLimit, nil
	}
	return limit, nil
}

func rollupConditions(alias string, filters queryFilters) (string, []any) {
	return filterConditions(alias, "bucket_start", filters)
}

func eventConditions(alias string, filters queryFilters) (string, []any) {
	return filterConditions(alias, "occurred_at", filters)
}

func filterConditions(alias, timeColumn string, filters queryFilters) (string, []any) {
	var b strings.Builder
	var args []any
	prefix := alias + "."
	if filters.IdentityKey != "" {
		b.WriteString(" AND " + prefix + "identity_key = ?")
		args = append(args, filters.IdentityKey)
	}
	if filters.DeviceID != "" {
		b.WriteString(" AND " + prefix + "device_id = ?")
		args = append(args, filters.DeviceID)
	}
	if filters.Model != "" {
		b.WriteString(" AND " + prefix + "model = ?")
		args = append(args, filters.Model)
	}
	if filters.From != nil {
		b.WriteString(" AND " + prefix + timeColumn + " >= ?")
		args = append(args, filters.From.Format(time.RFC3339Nano))
	}
	if filters.To != nil {
		b.WriteString(" AND " + prefix + timeColumn + " < ?")
		args = append(args, filters.To.Format(time.RFC3339Nano))
	}
	return b.String(), args
}

func breakdownColumn(groupBy string) (string, error) {
	switch groupBy {
	case "identity":
		return "r.identity_key", nil
	case "device":
		return "r.device_id", nil
	case "model":
		return "r.model", nil
	default:
		return "", fmt.Errorf("group_by must be identity, device, or model")
	}
}

func (s Server) countMissingPrices(r *http.Request, filters queryFilters) (int64, error) {
	conditions, args := eventConditions("e", filters)
	var count int64
	err := s.DB.QueryRowContext(r.Context(), `SELECT COUNT(*)
		FROM usage_events e
		WHERE e.pricing_status = 'missing'`+conditions, args...).Scan(&count)
	return count, err
}

func addTotals(total *UsageTotals, next UsageTotals) {
	total.InputTokens += next.InputTokens
	total.CacheReadTokens += next.CacheReadTokens
	total.CacheCreationTokens += next.CacheCreationTokens
	total.OutputTokens += next.OutputTokens
	total.TotalTokens += next.TotalTokens
	total.EventCount += next.EventCount
	if next.CostUSD == nil {
		return
	}
	if total.CostUSD == nil {
		value := *next.CostUSD
		total.CostUSD = &value
		return
	}
	*total.CostUSD += *next.CostUSD
}

func nullableFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	v := value.Float64
	return &v
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}

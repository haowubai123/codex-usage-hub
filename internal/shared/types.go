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
	BucketStart     time.Time `json:"bucket_start"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	CacheReadTokens int64     `json:"cache_read_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	CostUSD         *float64  `json:"cost_usd"`
	EventCount      int64     `json:"event_count"`
}

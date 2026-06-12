package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"codex-usage-tracker/internal/shared"
)

type Uploader struct {
	ServerURL string
	APIKey    string
	Client    *http.Client
}

type AuthError struct {
	StatusCode int
	Body       string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("ingest authorization failed with status %d", e.StatusCode)
}

type RetryableError struct {
	StatusCode int
	Body       string
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("ingest failed with retryable status %d", e.StatusCode)
}

func (u Uploader) Upload(ctx context.Context, req shared.IngestRequest) (shared.IngestResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return shared.IngestResponse{}, err
	}

	httpClient := u.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	endpoint := strings.TrimRight(u.ServerURL, "/") + "/api/v1/ingest"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return shared.IngestResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+u.APIKey)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return shared.IngestResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return shared.IngestResponse{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return shared.IngestResponse{}, &AuthError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if resp.StatusCode >= 500 {
		return shared.IngestResponse{}, &RetryableError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return shared.IngestResponse{}, fmt.Errorf("ingest failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var ingestResp shared.IngestResponse
	if err := json.Unmarshal(respBody, &ingestResp); err != nil {
		return shared.IngestResponse{}, err
	}
	return ingestResp, nil
}

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codex-usage-tracker/internal/shared"
)

func TestUploaderPostsAuthorizedIngestRequest(t *testing.T) {
	var gotAuth string
	var gotRequest shared.IngestRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/v1/ingest" {
			t.Fatalf("path = %q, want /api/v1/ingest", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(shared.IngestResponse{Accepted: len(gotRequest.Events)})
	}))
	defer server.Close()

	req := shared.IngestRequest{
		DeviceID:    "device-1",
		IdentityKey: "person-a",
		Platform:    "windows",
		Events: []shared.UsageEvent{{
			EventID:     "event-1",
			DeviceID:    "device-1",
			IdentityKey: "person-a",
			Source:      "codex_session",
			Model:       "gpt-5",
			InputTokens: 42,
			OccurredAt:  time.Unix(10, 0).UTC(),
		}},
	}
	uploader := Uploader{ServerURL: server.URL, APIKey: "secret", Client: server.Client()}

	resp, err := uploader.Upload(context.Background(), req)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if resp.Accepted != 1 {
		t.Fatalf("Accepted = %d, want 1", resp.Accepted)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q, want Bearer secret", gotAuth)
	}
	if gotRequest.DeviceID != req.DeviceID || gotRequest.IdentityKey != req.IdentityKey || gotRequest.Platform != req.Platform {
		t.Fatalf("request metadata = %#v, want %#v", gotRequest, req)
	}
	if len(gotRequest.Events) != 1 || gotRequest.Events[0].EventID != "event-1" || gotRequest.Events[0].InputTokens != 42 {
		t.Fatalf("request events = %#v, want event-1 with input tokens", gotRequest.Events)
	}
}

func TestUploaderReturnsTypedAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	uploader := Uploader{ServerURL: server.URL, APIKey: "bad", Client: server.Client()}
	_, err := uploader.Upload(context.Background(), shared.IngestRequest{})
	if err == nil {
		t.Fatal("Upload() error = nil, want auth error")
	}
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("Upload() error = %T, want *AuthError", err)
	}
	if authErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d, want 401", authErr.StatusCode)
	}
}

func TestUploaderReturnsRetryableErrorForServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "try again", http.StatusInternalServerError)
	}))
	defer server.Close()

	uploader := Uploader{ServerURL: server.URL, APIKey: "secret", Client: server.Client()}
	_, err := uploader.Upload(context.Background(), shared.IngestRequest{})
	if err == nil {
		t.Fatal("Upload() error = nil, want retryable error")
	}
	var retryableErr *RetryableError
	if !errors.As(err, &retryableErr) {
		t.Fatalf("Upload() error = %T, want *RetryableError", err)
	}
	if retryableErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want 500", retryableErr.StatusCode)
	}
}

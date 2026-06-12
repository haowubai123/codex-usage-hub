package server

import (
	"math"
	"testing"

	"codex-usage-tracker/internal/shared"
)

func TestCalculateCostUsesCacheInclusiveInput(t *testing.T) {
	usage := shared.UsageEvent{InputTokens: 1000, CacheReadTokens: 200, OutputTokens: 500}
	price := ModelPrice{InputPerMillion: 3, CacheReadPerMillion: 0.3, OutputPerMillion: 15}
	want := 0.0024 + 0.00006 + 0.0075

	got := CalculateCost(usage, price)
	if math.Abs(got-want) > 0.000000001 {
		t.Fatalf("CalculateCost() = %.12f, want %.12f", got, want)
	}
}

func TestCalculateCostClampsBillableInputAtZero(t *testing.T) {
	usage := shared.UsageEvent{InputTokens: 100, CacheReadTokens: 200, OutputTokens: 0}
	price := ModelPrice{InputPerMillion: 3, CacheReadPerMillion: 0.3}
	want := 0.00006

	got := CalculateCost(usage, price)
	if math.Abs(got-want) > 0.000000001 {
		t.Fatalf("CalculateCost() = %.12f, want %.12f", got, want)
	}
}

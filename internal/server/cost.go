package server

import "codex-usage-tracker/internal/shared"

type ModelPrice struct {
	Model                   string
	InputPerMillion         float64
	CacheReadPerMillion     float64
	CacheCreationPerMillion float64
	OutputPerMillion        float64
}

func CalculateCost(event shared.UsageEvent, price ModelPrice) float64 {
	billableInput := event.InputTokens - event.CacheReadTokens
	if billableInput < 0 {
		billableInput = 0
	}

	return (float64(billableInput)*price.InputPerMillion +
		float64(event.CacheReadTokens)*price.CacheReadPerMillion +
		float64(event.CacheCreationTokens)*price.CacheCreationPerMillion +
		float64(event.OutputTokens)*price.OutputPerMillion) / 1_000_000
}

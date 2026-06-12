package shared

import "testing"

func TestNormalizeModel(t *testing.T) {
	tests := map[string]string{
		"OpenAI/GPT-5.5-2026-05-14": "gpt-5.5",
		"gpt-5.4-20260514":          "gpt-5.4",
		"gpt-5.2-codex@low":         "gpt-5.2-codex-low",
		"glm-5.1":                   "glm-5.1",
	}
	for input, want := range tests {
		if got := NormalizeModel(input); got != want {
			t.Fatalf("NormalizeModel(%q) = %q, want %q", input, got, want)
		}
	}
}

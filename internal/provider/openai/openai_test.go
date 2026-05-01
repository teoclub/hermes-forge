package openai

import (
	"encoding/json"
	"testing"

	"github.com/teoclub/hermes-forge/internal/provider"
)

func TestRawJSONFromStringPreservesValidJSON(t *testing.T) {
	got := rawJSONFromString(`{"path":"README.md"}`)
	if !json.Valid(got) {
		t.Fatalf("rawJSONFromString() returned invalid JSON: %q", got)
	}
	if string(got) != `{"path":"README.md"}` {
		t.Fatalf("rawJSONFromString() = %s, want object unchanged", got)
	}
}

func TestRawJSONFromStringQuotesInvalidJSON(t *testing.T) {
	got := rawJSONFromString(`{path}`)
	if !json.Valid(got) {
		t.Fatalf("rawJSONFromString() returned invalid JSON: %q", got)
	}
	var value string
	if err := json.Unmarshal(got, &value); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if value != `{path}` {
		t.Fatalf("value = %q, want original invalid JSON text", value)
	}
}

func TestMergeConfigAppliesPerRequestOptions(t *testing.T) {
	p, err := NewOpenAIProvider(
		provider.WithModel("default-model"),
		provider.WithTemperature(0.7),
		provider.WithMaxTokens(100),
	)
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	cfg := p.mergeConfig(
		provider.WithModel("request-model"),
		provider.WithTemperature(0.2),
		provider.WithMaxTokens(50),
	)
	if cfg.Model != "request-model" || cfg.Temperature != 0.2 || cfg.MaxTokens != 50 {
		t.Fatalf("request config = %#v, want request overrides", cfg)
	}
	if p.cfg.Model != "default-model" || p.cfg.Temperature != 0.7 || p.cfg.MaxTokens != 100 {
		t.Fatalf("provider config mutated: %#v", p.cfg)
	}
}

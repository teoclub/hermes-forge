package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

func TestBuildMessageParamsIncludesRoleToolResults(t *testing.T) {
	p, err := NewAnthropicProvider(provider.WithModel("claude-test"))
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	params := p.buildMessageParams([]schema.Message{
		{
			Role:       schema.RoleTool,
			Content:    []schema.ContentPart{schema.Text("tool output")},
			ToolCallID: "call_123",
		},
	}, nil)

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal(params) error = %v", err)
	}
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Content   any    `json:"content"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(params) error = %v", err)
	}
	if len(payload.Messages) != 1 || len(payload.Messages[0].Content) != 1 {
		t.Fatalf("unexpected messages payload: %s", data)
	}
	block := payload.Messages[0].Content[0]
	if payload.Messages[0].Role != "user" || block.Type != "tool_result" || block.ToolUseID != "call_123" || !strings.Contains(string(data), "tool output") {
		t.Fatalf("unexpected tool result payload: %s", data)
	}
}

func TestBuildMessageParamsAppliesPerRequestOptions(t *testing.T) {
	p, err := NewAnthropicProvider(
		provider.WithModel("claude-default"),
		provider.WithTemperature(0.7),
		provider.WithMaxTokens(100),
	)
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	params := p.buildMessageParams(nil, nil,
		provider.WithModel("claude-request"),
		provider.WithTemperature(0.2),
		provider.WithMaxTokens(50),
	)

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal(params) error = %v", err)
	}
	var payload struct {
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int64   `json:"max_tokens"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(params) error = %v", err)
	}
	if payload.Model != "claude-request" || payload.Temperature != 0.2 || payload.MaxTokens != 50 {
		t.Fatalf("payload = %+v, want request overrides", payload)
	}
	if p.cfg.Model != "claude-default" || p.cfg.Temperature != 0.7 || p.cfg.MaxTokens != 100 {
		t.Fatalf("provider config mutated: %#v", p.cfg)
	}
}

func TestAnthropicToolInputSchemaNormalizesRequiredAnySlice(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []any{"path"},
	}

	got := anthropicToolInputSchema(input)
	if len(got.Required) != 1 || got.Required[0] != "path" {
		t.Fatalf("required = %#v, want [path]", got.Required)
	}
	properties, ok := got.Properties.(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T, want map[string]any", got.Properties)
	}
	if _, ok := properties["path"]; !ok {
		t.Fatalf("properties = %#v, want path", got.Properties)
	}
}

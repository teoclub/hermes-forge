package anthropic

import (
	"context"
	"errors"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/teoclub/hermes-forge/internal/client"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ provider.LLMProvider = (*AnthropicProvider)(nil)

type AnthropicProvider struct {
	client anthropic.Client
	cfg    client.Config
}

func NewAnthropicProvider(opts ...client.Option) (*AnthropicProvider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, errors.New("请设置 ANTHROPIC_API_KEY 环境变量")
	}

	cfg := client.Config{
		Model: "claude-3-5-sonnet-latest",
	}
	client.Apply(&cfg, opts...)

	if cfg.BaseURL == "" {
		cfg.BaseURL = os.Getenv("ANTHROPIC_BASE_URL")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}

	return &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(cfg.BaseURL)),
		cfg:    cfg,
	}, nil
}

func (a *AnthropicProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	panic("unimplemented")
}

func (a *AnthropicProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (<-chan *schema.StreamChunk, error) {
	panic("unimplemented")
}

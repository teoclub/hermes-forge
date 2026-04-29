package anthropic

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ provider.LLMProvider = (*AnthropicProvider)(nil)

type AnthropicProvider struct {
	client anthropic.Client
	cfg    provider.Config
}

func NewAnthropicProvider(opts ...provider.Option) (*AnthropicProvider, error) {
	cfg := provider.NewConfig(opts...)
	return &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(cfg.BaseURL)),
		cfg:    cfg,
	}, nil
}

func (a *AnthropicProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (*schema.Message, error) {
	return nil, provider.WrapError("anthropic", "generate", provider.ErrNotImplemented)
}

func (a *AnthropicProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (<-chan *schema.StreamChunk, error) {
	return nil, provider.WrapError("anthropic", "stream", provider.ErrNotImplemented)
}

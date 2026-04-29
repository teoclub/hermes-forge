package openai

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ provider.LLMProvider = (*OpenAIProvider)(nil)

type OpenAIProvider struct {
	client openai.Client
	cfg    provider.Config
}

func NewOpenAIProvider(opts ...provider.Option) (*OpenAIProvider, error) {
	cfg := provider.NewConfig(opts...)
	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(cfg.BaseURL)),
		cfg:    cfg,
	}, nil
}

func (o *OpenAIProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (*schema.Message, error) {
	return nil, provider.WrapError("openai", "generate", provider.ErrNotImplemented)
}

func (o *OpenAIProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (<-chan *schema.StreamChunk, error) {
	return nil, provider.WrapError("openai", "stream", provider.ErrNotImplemented)
}

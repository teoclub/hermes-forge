package ollama

import (
	"context"

	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ provider.LLMProvider = (*OllamaProvider)(nil)

type OllamaProvider struct {
	cfg provider.Config
}

func (o *OllamaProvider) Name() string {
	return "ollama"
}

func NewOllamaProvider(opts ...provider.Option) (*OllamaProvider, error) {
	cfg := provider.NewConfig(opts...)
	return &OllamaProvider{
		cfg: cfg,
	}, nil
}

func (o *OllamaProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (*schema.Response, error) {
	return nil, provider.WrapError("ollama", "generate", provider.ErrNotImplemented)
}

func (o *OllamaProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (<-chan *schema.StreamChunk, error) {
	return nil, provider.WrapError("ollama", "stream", provider.ErrNotImplemented)
}

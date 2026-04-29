package ollama

import (
	"context"
	"github.com/teoclub/hermes-forge/internal/client"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ provider.LLMProvider = (*OllamaProvider)(nil)

type OllamaProvider struct {
	cfg client.Config
}

func NewOllamaProvider(opts ...client.Option) (*OllamaProvider, error) {
	cfg := client.Config{
		BaseURL: "http://localhost:11434",
		Model:   "qwen2.5-coder:7b",
	}
	client.Apply(&cfg, opts...)

	return &OllamaProvider{cfg: cfg}, nil
}

func (o *OllamaProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	panic("unimplemented")
}

func (o *OllamaProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (<-chan *schema.StreamChunk, error) {
	panic("unimplemented")
}

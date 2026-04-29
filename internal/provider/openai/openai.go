package openai

import (
	"context"
	"errors"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/teoclub/hermes-forge/internal/client"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ provider.LLMProvider = (*OpenAIProvider)(nil)

type OpenAIProvider struct {
	client openai.Client
	cfg    client.Config
}

func NewOpenAIProvider(opts ...client.Option) (*OpenAIProvider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("请设置 OPENAI_API_KEY 环境变量")
	}

	cfg := client.Config{
		Model: "gpt-4o-mini",
	}
	client.Apply(&cfg, opts...)

	if cfg.BaseURL == "" {
		cfg.BaseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}

	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(cfg.BaseURL)),
		cfg:    cfg,
	}, nil
}

func (o *OpenAIProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	panic("unimplemented")
}

func (o *OpenAIProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (<-chan *schema.StreamChunk, error) {
	panic("unimplemented")
}

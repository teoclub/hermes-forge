package provider

import (
	"context"

	"github.com/teoclub/hermes-forge/internal/schema"
)

// LLMProvider defines the normalized model backend contract.
type LLMProvider interface {
	Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...Option) (*schema.Message, error)
	Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...Option) (<-chan *schema.StreamChunk, error)
}

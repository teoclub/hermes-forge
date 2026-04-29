package provider

import (
	"context"

	"github.com/teoclub/hermes-forge/internal/schema"
)

// LLMProvider defines the unified interface for communicating with large models
type LLMProvider interface {
	// Generate receives the current context history and available tools list, returns the model response
	Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error)

	// Stream receives the current context history and available tools list, returns a channel of streaming responses
	Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition) (<-chan *schema.StreamChunk, error)
}

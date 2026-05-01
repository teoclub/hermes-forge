package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

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

func (a *AnthropicProvider) Name() string {
	return "anthropic"
}

func (a *AnthropicProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (*schema.Response, error) {
	params := a.buildMessageParams(prompt, availableTools, opts...)

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, provider.WrapError("anthropic", "generate", fmt.Errorf("request failed: %w", err))
	}

	resultMsg := schema.Message{
		Role: schema.RoleAssistant,
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			resultMsg.Append(schema.Text(block.Text))
		case "thinking":
			resultMsg.Append(schema.Reasoning(block.Thinking))
		case "tool_use":
			resultMsg.ToolCalls = append(resultMsg.ToolCalls, schema.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	usage := schema.UsageMetadata{
		PromptTokens:     int(resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens),
		CompletionTokens: int(resp.Usage.OutputTokens),
		TotalTokens:      int(resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.OutputTokens),
	}

	return &schema.Response{
		ID:           resp.ID,
		Model:        string(resp.Model),
		Message:      resultMsg,
		Usage:        usage,
		FinishReason: string(resp.StopReason),
		Raw:          resp,
	}, nil
}

func (a *AnthropicProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (<-chan *schema.StreamChunk, error) {
	stream := a.client.Messages.NewStreaming(ctx, a.buildMessageParams(prompt, availableTools, opts...))
	if err := stream.Err(); err != nil {
		return nil, provider.WrapError("anthropic", "stream", fmt.Errorf("request failed: %w", err))
	}

	ch := make(chan *schema.StreamChunk)
	go func() {
		defer stream.Close()
		defer close(ch)

		for stream.Next() {
			event := stream.Current()
			switch event := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				var chunk *schema.StreamChunk
				switch delta := event.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					if delta.Text != "" {
						chunk = &schema.StreamChunk{Delta: delta.Text}
					}
				case anthropic.ThinkingDelta:
					if delta.Thinking != "" {
						chunk = &schema.StreamChunk{ReasoningDelta: delta.Thinking}
					}
				}
				if chunk == nil {
					continue
				}
				if !sendStreamChunk(ctx, ch, chunk) {
					return
				}
			case anthropic.MessageStopEvent:
				sendStreamChunk(ctx, ch, &schema.StreamChunk{Done: true})
				return
			}
		}

		if err := stream.Err(); err != nil {
			sendStreamChunk(ctx, ch, &schema.StreamChunk{
				Err:  provider.WrapError("anthropic", "stream", err),
				Done: true,
			})
		}
	}()

	return ch, nil
}

func (a *AnthropicProvider) buildMessageParams(prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) anthropic.MessageNewParams {
	cfg := a.mergeConfig(opts...)
	var anthropicMsgs []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range prompt {
		switch msg.Role {
		case schema.RoleSystem:
			systemPrompt = schema.MessageText(msg.Content)
		case schema.RoleUser:
			if msg.ToolCallID != "" {
				anthropicMsgs = append(anthropicMsgs, anthropicToolResultMessage(msg))
				continue
			}

			blocks := anthropicContentBlocks(msg.Content)
			if len(blocks) > 0 {
				anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(blocks...))
			}
		case schema.RoleTool:
			if msg.ToolCallID != "" {
				anthropicMsgs = append(anthropicMsgs, anthropicToolResultMessage(msg))
			}
		case schema.RoleAssistant:
			blocks := anthropicContentBlocks(msg.Content)
			for _, toolCall := range msg.ToolCalls {
				var input any
				if len(toolCall.Arguments) > 0 {
					if err := json.Unmarshal(toolCall.Arguments, &input); err != nil {
						input = toolCall.Arguments
					}
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(toolCall.ID, input, toolCall.Name))
			}
			if len(blocks) > 0 {
				anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(blocks...))
			}
		}
	}

	anthropicTools := make([]anthropic.ToolUnionParam, 0, len(availableTools))
	for _, toolDef := range availableTools {
		tp := anthropic.ToolParam{
			Name:        toolDef.Name,
			Description: anthropic.String(toolDef.Description),
			InputSchema: anthropicToolInputSchema(toolDef.InputSchema),
		}
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{OfTool: &tp})
	}

	maxTokens := int64(cfg.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(cfg.Model),
		MaxTokens:   maxTokens,
		Messages:    anthropicMsgs,
		Temperature: anthropic.Float(cfg.Temperature),
	}
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
	}
	if len(anthropicTools) > 0 {
		params.Tools = anthropicTools
	}

	return params
}

func (a *AnthropicProvider) mergeConfig(opts ...provider.Option) provider.Config {
	cfg := a.cfg.Clone()
	provider.Apply(&cfg, opts...)
	if cfg.Model == "" {
		cfg.Model = a.cfg.Model
	}
	if cfg.APIKey == "" {
		cfg.APIKey = a.cfg.APIKey
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = a.cfg.BaseURL
	}
	return cfg
}

func anthropicToolResultMessage(msg schema.Message) anthropic.MessageParam {
	return anthropic.NewUserMessage(
		anthropic.NewToolResultBlock(msg.ToolCallID, schema.MessageText(msg.Content), false),
	)
}

func sendStreamChunk(ctx context.Context, ch chan<- *schema.StreamChunk, chunk *schema.StreamChunk) bool {
	select {
	case <-ctx.Done():
		select {
		case ch <- &schema.StreamChunk{Err: ctx.Err(), Done: true}:
		default:
		}
		return false
	case ch <- chunk:
		return true
	}
}

func anthropicContentBlocks(parts []schema.ContentPart) []anthropic.ContentBlockParamUnion {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(parts))
	for _, part := range parts {
		switch p := part.(type) {
		case *schema.TextContent:
			if p.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(p.Text))
			}
		case schema.TextContent:
			if p.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(p.Text))
			}
		case *schema.ImageContent:
			if p.URL != "" {
				blocks = append(blocks, anthropic.NewImageBlock(anthropic.URLImageSourceParam{URL: p.URL}))
			}
		}
	}
	return blocks
}

func anthropicToolInputSchema(input any) anthropic.ToolInputSchemaParam {
	schemaMap := mapFromSchema(input)
	if schemaMap == nil {
		return anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		}
	}

	properties, _ := schemaMap["properties"].(map[string]any)
	required := stringSlice(schemaMap["required"])

	if properties == nil {
		properties = make(map[string]any)
		for name, raw := range schemaMap {
			if name == "type" || name == "required" || name == "properties" {
				continue
			}
			if field, ok := raw.(map[string]any); ok {
				field = copyMap(field)
				if isRequired, ok := field["required"].(bool); ok && isRequired {
					required = append(required, name)
					delete(field, "required")
				}
				properties[name] = field
				continue
			}
			properties[name] = raw
		}
	}

	return anthropic.ToolInputSchemaParam{
		Properties: properties,
		Required:   required,
	}
}

func mapFromSchema(input any) map[string]any {
	switch m := input.(type) {
	case map[string]any:
		return m
	default:
		if input == nil {
			return nil
		}
		data, err := json.Marshal(input)
		if err != nil {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

func copyMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stringSlice(input any) []string {
	switch values := input.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ provider.LLMProvider = (*OpenAIProvider)(nil)

type OpenAIProvider struct {
	client openai.Client
	cfg    provider.Config
}

func (o *OpenAIProvider) Name() string {
	return "openai"
}

func NewOpenAIProvider(opts ...provider.Option) (*OpenAIProvider, error) {
	cfg := provider.NewConfig(opts...)
	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(cfg.BaseURL)),
		cfg:    cfg,
	}, nil
}

func (o *OpenAIProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (*schema.Response, error) {
	cfg := o.mergeConfig(opts...)
	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(cfg.Model),
		Messages:    openaiMessages(prompt),
		Temperature: openai.Float(cfg.Temperature),
	}
	if cfg.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(cfg.MaxTokens))
	}
	if tools := openaiTools(availableTools); len(tools) > 0 {
		params.Tools = tools
	}

	resp, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, provider.WrapError("openai", "generate", fmt.Errorf("request failed: %w", err))
	}
	if len(resp.Choices) == 0 {
		return nil, provider.WrapError("openai", "generate", fmt.Errorf("empty choices"))
	}

	choice := resp.Choices[0]
	resultMsg := schema.Message{
		Role: schema.RoleAssistant,
	}
	if choice.Message.Content != "" {
		resultMsg.Append(schema.Text(choice.Message.Content))
	}
	for _, tc := range choice.Message.ToolCalls {
		if tc.Type != "function" {
			continue
		}
		resultMsg.ToolCalls = append(resultMsg.ToolCalls, schema.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: rawJSONFromString(tc.Function.Arguments),
		})
	}

	usage := schema.UsageMetadata{
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
		TotalTokens:      int(resp.Usage.TotalTokens),
	}

	return &schema.Response{
		ID:           resp.ID,
		Model:        resp.Model,
		Message:      resultMsg,
		Usage:        usage,
		FinishReason: choice.FinishReason,
		Raw:          resp,
	}, nil
}

func (o *OpenAIProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (<-chan *schema.StreamChunk, error) {
	return nil, provider.WrapError("openai", "stream", provider.ErrNotImplemented)
}

func (o *OpenAIProvider) mergeConfig(opts ...provider.Option) provider.Config {
	cfg := o.cfg.Clone()
	provider.Apply(&cfg, opts...)
	return cfg
}

func openaiMessages(prompt []schema.Message) []openai.ChatCompletionMessageParamUnion {
	openaiMsgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(prompt))

	for _, msg := range prompt {
		text := schema.MessageText(msg.Content)
		switch msg.Role {
		case schema.RoleSystem:
			openaiMsgs = append(openaiMsgs, openai.SystemMessage(text))
		case schema.RoleUser:
			if msg.ToolCallID != "" {
				// 注意：这里需要使用 openai.ToolMessage 来创建工具消息， 参数顺序是 (content, tool_call_id)
				openaiMsgs = append(openaiMsgs, openai.ToolMessage(text, msg.ToolCallID))
				continue
			}
			if parts := openaiContentParts(msg.Content); len(parts) > 0 {
				openaiMsgs = append(openaiMsgs, openai.UserMessage(parts))
			} else {
				openaiMsgs = append(openaiMsgs, openai.UserMessage(text))
			}
		case schema.RoleTool:
			if msg.ToolCallID != "" {
				openaiMsgs = append(openaiMsgs, openai.ToolMessage(text, msg.ToolCallID))
			}
		case schema.RoleAssistant:
			assistant := openai.ChatCompletionAssistantMessageParam{}
			if text != "" {
				// assistant.Content.OfString = openai.String(text)
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(text),
				}
			}

			if len(msg.ToolCalls) > 0 {
				var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID:   tc.ID,
							Type: "function",
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(tc.Arguments),
							},
						},
					})
				}
				assistant.ToolCalls = toolCalls
			}

			openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistant,
			})

		}
	}

	return openaiMsgs
}

func openaiContentParts(parts []schema.ContentPart) []openai.ChatCompletionContentPartUnionParam {
	openaiParts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		switch p := part.(type) {
		case *schema.TextContent:
			if p.Text != "" {
				openaiParts = append(openaiParts, openai.ChatCompletionContentPartUnionParam{
					OfText: &openai.ChatCompletionContentPartTextParam{Text: p.Text},
				})
			}
		case schema.TextContent:
			if p.Text != "" {
				openaiParts = append(openaiParts, openai.ChatCompletionContentPartUnionParam{
					OfText: &openai.ChatCompletionContentPartTextParam{Text: p.Text},
				})
			}
		case *schema.ImageContent:
			if p.URL != "" {
				openaiParts = append(openaiParts, openai.ChatCompletionContentPartUnionParam{
					OfImageURL: &openai.ChatCompletionContentPartImageParam{
						ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
							URL:    p.URL,
							Detail: p.Detail,
						},
					},
				})
			}
		}
	}
	return openaiParts
}

func openaiTools(availableTools []schema.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(availableTools))
	for _, toolDef := range availableTools {
		tools = append(tools, openai.ChatCompletionFunctionTool(
			shared.FunctionDefinitionParam{
				Name:        toolDef.Name,
				Description: openai.String(toolDef.Description),
				Parameters:  openaiFunctionParameters(toolDef.InputSchema),
			},
		))
	}
	return tools
}

func openaiFunctionParameters(input any) shared.FunctionParameters {
	switch v := input.(type) {
	case map[string]any:
		return shared.FunctionParameters(v)
	default:
		if input == nil {
			return nil
		}
		data, err := json.Marshal(input)
		if err != nil {
			return nil
		}
		var out shared.FunctionParameters
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

func rawJSONFromString(input string) json.RawMessage {
	if json.Valid([]byte(input)) {
		return json.RawMessage(input)
	}
	data, err := json.Marshal(input)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return data
}

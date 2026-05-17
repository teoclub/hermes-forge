package schema

import (
	"encoding/json"
	"strings"
)

// Role represents the speaker for a given message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPart is implemented by any structure that can be included inside a message.
type ContentPart interface {
	Type() string
}

// Message represents one turn in a conversation with the provider and can mix modalities.
type Message struct {
	Role       Role          `json:"role"`
	Content    []ContentPart `json:"content"`
	ToolCalls  []ToolCall    `json:"tool_calls"`
	ToolCallID string        `json:"tool_call_id"`
}

// ToolCall is a provider-requested tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult is the result of a tool invocation.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
}

// ToolDefinition describes a tool that can be invoked by the agent.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

// Append adds one or more content parts to the message.
func (m *Message) Append(parts ...ContentPart) {
	m.Content = append(m.Content, parts...)
}

// NewMessage constructs a message with the provided role and content parts.
func NewMessage(role Role, parts ...ContentPart) *Message {
	return &Message{Role: role, Content: append([]ContentPart(nil), parts...)}
}

// NewSystemMessage returns a message authored by the system.
func NewSystemMessage(parts ...ContentPart) *Message {
	return NewMessage(RoleSystem, parts...)
}

// NewUserMessage returns a message authored by the end-user.
func NewUserMessage(parts ...ContentPart) *Message {
	return NewMessage(RoleUser, parts...)
}

// NewAssistantMessage returns a message authored by the assistant.
func NewAssistantMessage(parts ...ContentPart) *Message {
	return NewMessage(RoleAssistant, parts...)
}

// NewToolMessage returns a message authored by a tool.
func NewToolMessage(parts ...ContentPart) *Message {
	return NewMessage(RoleTool, parts...)
}

type TextContent struct {
	Text string `json:"text"`
}

func (t TextContent) Type() string {
	return "text"
}

func Text(s string) *TextContent {
	return &TextContent{Text: s}
}

func MessageText(parts []ContentPart) string {
	if len(parts) == 0 {
		return ""
	}

	var b strings.Builder
	first := true
	for _, p := range parts {
		if t, ok := p.(*TextContent); ok {
			if !first {
				b.WriteString("\n")
			}
			b.WriteString(t.Text)
			first = false
		}
	}
	return b.String()
}

// ReasoningContent represents model-internal reasoning or thinking text when providers expose it.
type ReasoningContent struct {
	Text string
}

// Type identifies the piece as reasoning.
func (r *ReasoningContent) Type() string { return "reasoning" }

// Reasoning is a helper constructor for a reasoning part.
func Reasoning(s string) *ReasoningContent { return &ReasoningContent{Text: s} }

// ImageContent represents a reference to an image by URL or base64 payload.
type ImageContent struct {
	URL       string
	Base64    string
	MediaType string
	Detail    string // optional granularity instruction used by some providers
}

// Type identifies the piece as an image.
func (i *ImageContent) Type() string { return "image" }

// Image is a helper constructor for an image part.
func Image(url string) *ImageContent { return &ImageContent{URL: url} }

// ImageData is a helper constructor for an inline base64-encoded image.
func ImageData(mediaType, data string) *ImageContent {
	return &ImageContent{MediaType: mediaType, Base64: data}
}

// UsageMetadata captures token consumption and cost information reported by providers.
type UsageMetadata struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Cost             float64 // Estimated cost in US dollars based on configured per-million-token rates.
}

// Response contains the normalized result returned from a provider.
type Response struct {
	ID           string
	Model        string
	Message      Message
	Usage        UsageMetadata
	FinishReason string
	Raw          any
}

// Text concatenates the textual segments of the response for the common use case where only text matters.
func (r *Response) Text() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range r.Message.Content {
		if tc, ok := part.(*TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// ReasoningText concatenates reasoning segments exposed by providers.
func (r *Response) ReasoningText() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range r.Message.Content {
		if rc, ok := part.(*ReasoningContent); ok {
			b.WriteString(rc.Text)
		}
	}
	return b.String()
}

// StreamChunk represents a partial streaming response.
type StreamChunk struct {
	Delta          string
	ReasoningDelta string
	Done           bool
	Err            error
}

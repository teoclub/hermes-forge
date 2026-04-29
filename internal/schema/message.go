package schema

import (
	"encoding/json"
	"strings"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPart represents a part of the message content.
type ContentPart interface {
	Type() string
}

// Message represents one dialogue turn in the agent context.
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

func Text(text string) TextContent {
	return TextContent{Text: text}
}

func MessageText(parts []ContentPart) string {
	if len(parts) == 0 {
		return ""
	}

	var b strings.Builder
	first := true
	for _, p := range parts {
		if t, ok := p.(TextContent); ok {
			if !first {
				b.WriteString("\n")
			}
			b.WriteString(t.Text)
			first = false
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

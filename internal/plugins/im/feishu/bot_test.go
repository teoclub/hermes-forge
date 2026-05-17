package feishu

import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestResolveConnectionMode(t *testing.T) {
	tests := map[string]string{
		"":          connectionModeWebSocket,
		"websocket": connectionModeWebSocket,
		"ws":        connectionModeWebSocket,
		" WEBHOOK ": connectionModeWebhook,
		"http":      connectionModeWebhook,
		"custom":    "custom",
	}

	for input, want := range tests {
		if got := resolveConnectionMode(input); got != want {
			t.Fatalf("resolveConnectionMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExtractTextContent(t *testing.T) {
	got, err := extractTextContent(newMessageEvent("chat-1", `{"text":"hello"}`))
	if err != nil {
		t.Fatalf("extractTextContent returned error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("content = %q, want hello", got)
	}
}

func TestExtractTextContentFallback(t *testing.T) {
	got, err := extractTextContent(newMessageEvent("chat-1", `{"text":"hello`))
	if err != nil {
		t.Fatalf("extractTextContent fallback returned error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("content = %q, want hello", got)
	}
}

func TestExtractTextContentRequiresChatID(t *testing.T) {
	_, err := extractTextContent(newMessageEvent("", `{"text":"hello"}`))
	if err == nil {
		t.Fatal("extractTextContent returned nil error, want missing chat_id error")
	}
}

func TestExtractTextContentRequiresMessage(t *testing.T) {
	_, err := extractTextContent(&larkim.P2MessageReceiveV1{})
	if err == nil {
		t.Fatal("extractTextContent returned nil error, want empty event error")
	}
}

func newMessageEvent(chatID, content string) *larkim.P2MessageReceiveV1 {
	var chatPtr *string
	if chatID != "" {
		chatPtr = strPtr(chatID)
	}
	return &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				ChatId:  chatPtr,
				Content: strPtr(content),
			},
		},
	}
}

func strPtr(s string) *string {
	return &s
}

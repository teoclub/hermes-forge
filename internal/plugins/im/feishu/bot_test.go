package feishu

import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	pluginim "github.com/teoclub/hermes-forge/internal/plugins/im"
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
	got, err := extractTextContent(newMessageEvent("chat-1", "text", `{"text":"hello"}`))
	if err != nil {
		t.Fatalf("extractTextContent returned error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("content = %q, want hello", got)
	}
}

func TestExtractTextContentFallback(t *testing.T) {
	got, err := extractTextContent(newMessageEvent("chat-1", "text", `{"text":"hello`))
	if err != nil {
		t.Fatalf("extractTextContent fallback returned error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("content = %q, want hello", got)
	}
}

func TestExtractTextContentRequiresChatID(t *testing.T) {
	_, err := extractTextContent(newMessageEvent("", "text", `{"text":"hello"}`))
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

func TestExtractIncomingMessageStripsGroupMention(t *testing.T) {
	event := newMessageEvent("chat-1", "text", `{"text":"@_user_ hello"}`)
	event.Event.Message.ChatType = strPtr("group")

	got, err := extractIncomingMessage(event)
	if err != nil {
		t.Fatalf("extractIncomingMessage returned error: %v", err)
	}
	if got.Content != "hello" {
		t.Fatalf("Content = %q, want hello", got.Content)
	}
	if got.ChatType != pluginim.ChatTypeGroup {
		t.Fatalf("ChatType = %q, want group", got.ChatType)
	}
}

func TestExtractIncomingMessageFile(t *testing.T) {
	got, err := extractIncomingMessage(newMessageEvent("chat-1", "file", `{"file_key":"file-1","file_name":"report.pdf"}`))
	if err != nil {
		t.Fatalf("extractIncomingMessage returned error: %v", err)
	}
	if got.MessageType != pluginim.MessageTypeFile {
		t.Fatalf("MessageType = %q, want file", got.MessageType)
	}
	if got.FileKey != "file-1" || got.FileName != "report.pdf" {
		t.Fatalf("file = (%q, %q), want (file-1, report.pdf)", got.FileKey, got.FileName)
	}
}

func TestExtractIncomingMessageImage(t *testing.T) {
	got, err := extractIncomingMessage(newMessageEvent("chat-1", "image", `{"image_key":"img-1"}`))
	if err != nil {
		t.Fatalf("extractIncomingMessage returned error: %v", err)
	}
	if got.MessageType != pluginim.MessageTypeImage {
		t.Fatalf("MessageType = %q, want image", got.MessageType)
	}
	if got.FileKey != "img-1" || got.FileName != "img-1.png" {
		t.Fatalf("image = (%q, %q), want (img-1, img-1.png)", got.FileKey, got.FileName)
	}
}

func TestExtractIncomingMessagePost(t *testing.T) {
	content := `{"title":"Title","content":[[{"tag":"text","text":"hello "},{"tag":"a","text":"link"}],[{"tag":"at","text":"ignored"}]]}`
	got, err := extractIncomingMessage(newMessageEvent("chat-1", "post", content))
	if err != nil {
		t.Fatalf("extractIncomingMessage returned error: %v", err)
	}
	if got.MessageType != pluginim.MessageTypeText {
		t.Fatalf("MessageType = %q, want text", got.MessageType)
	}
	if got.Content != "Title\nhello link" {
		t.Fatalf("Content = %q, want Title\\nhello link", got.Content)
	}
}

func newMessageEvent(chatID, messageType, content string) *larkim.P2MessageReceiveV1 {
	var chatPtr *string
	if chatID != "" {
		chatPtr = strPtr(chatID)
	}
	var typePtr *string
	if messageType != "" {
		typePtr = strPtr(messageType)
	}
	return &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				ChatId:      chatPtr,
				MessageId:   strPtr("msg-1"),
				MessageType: typePtr,
				Content:     strPtr(content),
			},
		},
	}
}

func strPtr(s string) *string {
	return &s
}

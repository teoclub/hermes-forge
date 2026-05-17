package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseMessageText(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:    42,
		MessageType:  1,
		FromUserID:   "user-1",
		ContextToken: "ctx-1",
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: " hello "},
			},
		},
	})

	if got == nil {
		t.Fatal("parseMessage returned nil")
	}
	if got.UserID != "user-1" {
		t.Fatalf("UserID = %q, want user-1", got.UserID)
	}
	if got.Content != "hello" {
		t.Fatalf("Content = %q, want hello", got.Content)
	}
	if got.MessageID != "42" {
		t.Fatalf("MessageID = %q, want 42", got.MessageID)
	}
	if got.ContextToken != "ctx-1" {
		t.Fatalf("ContextToken = %q, want ctx-1", got.ContextToken)
	}
}

func TestParseMessageVoiceText(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:   43,
		MessageType: 1,
		FromUserID:  "user-2",
		ItemList: []messageItem{
			{
				Type:      3,
				VoiceItem: &voiceItem{Text: "voice text"},
			},
		},
	})

	if got == nil {
		t.Fatal("parseMessage returned nil")
	}
	if got.Content != "voice text" {
		t.Fatalf("Content = %q, want voice text", got.Content)
	}
}

func TestParseMessageSkipsBotMessage(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:   44,
		MessageType: 2,
		FromUserID:  "bot",
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: "ignore me"},
			},
		},
	})

	if got != nil {
		t.Fatalf("parseMessage returned %#v, want nil", got)
	}
}

func TestParseMessageSkipsEmptyContent(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:   45,
		MessageType: 1,
		FromUserID:  "user-3",
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: "   "},
			},
		},
	})

	if got != nil {
		t.Fatalf("parseMessage returned %#v, want nil", got)
	}
}

func TestPollReconnectDelay(t *testing.T) {
	tests := map[int]time.Duration{
		0: reconnectBaseDelay,
		1: reconnectBaseDelay,
		2: 2 * reconnectBaseDelay,
		5: 16 * reconnectBaseDelay,
	}

	for attempt, want := range tests {
		if got := pollReconnectDelay(attempt); got != want {
			t.Fatalf("pollReconnectDelay(%d) = %s, want %s", attempt, got, want)
		}
	}

	if got := pollReconnectDelay(100); got != reconnectMaxDelay {
		t.Fatalf("pollReconnectDelay(100) = %s, want %s", got, reconnectMaxDelay)
	}
}

func TestWeChatReporterSendsText(t *testing.T) {
	var captured requestCapture
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured.method = req.Method
		captured.path = req.URL.Path
		captured.authType = req.Header.Get("AuthorizationType")
		captured.auth = req.Header.Get("Authorization")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &captured.body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ret":0}`))),
			Header:     make(http.Header),
		}, nil
	})}

	reporter := NewWeChatReporter("token-1", "user-1", "ctx-1", client)
	reporter.OnMessage(context.Background(), "hello")

	if captured.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", captured.method)
	}
	if captured.path != "/ilink/bot/sendmessage" {
		t.Fatalf("path = %s, want /ilink/bot/sendmessage", captured.path)
	}
	if captured.authType != "ilink_bot_token" {
		t.Fatalf("AuthorizationType = %q, want ilink_bot_token", captured.authType)
	}
	if captured.auth != "Bearer token-1" {
		t.Fatalf("Authorization = %q, want Bearer token-1", captured.auth)
	}

	msg := captured.body["msg"].(map[string]interface{})
	if msg["to_user_id"] != "user-1" {
		t.Fatalf("to_user_id = %v, want user-1", msg["to_user_id"])
	}
	if msg["context_token"] != "ctx-1" {
		t.Fatalf("context_token = %v, want ctx-1", msg["context_token"])
	}
	items := msg["item_list"].([]interface{})
	textItem := items[0].(map[string]interface{})["text_item"].(map[string]interface{})
	if text := textItem["text"].(string); !strings.Contains(text, "hello") {
		t.Fatalf("text = %q, want hello", text)
	}
}

type requestCapture struct {
	method   string
	path     string
	authType string
	auth     string
	body     map[string]interface{}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseCallbackMessageGroupText(t *testing.T) {
	got := parseCallbackMessage(&callbackMessage{
		ConversationID:   "cid-1",
		ConversationType: conversationTypeGroup,
		MsgID:            "msg-1",
		Text:             &textContent{Content: " hello "},
		SenderStaffID:    "staff-1",
		SenderNick:       "Neo",
		SessionWebhook:   "https://example.test/webhook",
	})

	if got == nil {
		t.Fatal("parseCallbackMessage returned nil")
	}
	if got.UserID != "staff-1" {
		t.Fatalf("UserID = %q, want staff-1", got.UserID)
	}
	if got.ConversationID != "cid-1" {
		t.Fatalf("ConversationID = %q, want cid-1", got.ConversationID)
	}
	if got.Content != "hello" {
		t.Fatalf("Content = %q, want hello", got.Content)
	}
	if got.SessionWebhook == "" {
		t.Fatal("SessionWebhook is empty")
	}
}

func TestParseCallbackMessageDirectUsesSender(t *testing.T) {
	got := parseCallbackMessage(&callbackMessage{
		ConversationID:   "cid-1",
		ConversationType: "1",
		MsgID:            "msg-1",
		Text:             &textContent{Content: "ping"},
		SenderID:         "sender-1",
	})

	if got == nil {
		t.Fatal("parseCallbackMessage returned nil")
	}
	if got.UserID != "sender-1" {
		t.Fatalf("UserID = %q, want sender-1", got.UserID)
	}
	if got.ConversationID != "sender-1" {
		t.Fatalf("ConversationID = %q, want sender-1", got.ConversationID)
	}
}

func TestParseCallbackMessageIgnoresEmptyText(t *testing.T) {
	got := parseCallbackMessage(&callbackMessage{Text: &textContent{Content: "   "}})
	if got != nil {
		t.Fatalf("parseCallbackMessage returned %#v, want nil", got)
	}
}

func TestVerifyCallback(t *testing.T) {
	secret := "secret"
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("Timestamp", ts)
	req.Header.Set("Sign", dingtalkSign(ts, secret))

	bot := &DingTalkBot{clientSecret: secret}
	if err := bot.verifyCallback(req); err != nil {
		t.Fatalf("verifyCallback returned error: %v", err)
	}
}

func TestVerifyCallbackRejectsBadSignature(t *testing.T) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("Timestamp", ts)
	req.Header.Set("Sign", "bad")

	bot := &DingTalkBot{clientSecret: "secret"}
	if err := bot.verifyCallback(req); err == nil {
		t.Fatal("verifyCallback returned nil error, want invalid signature")
	}
}

func TestDingTalkReporterSendsMarkdown(t *testing.T) {
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reporter := NewDingTalkReporter(server.URL, server.Client())
	reporter.OnToolCall(context.Background(), "bash", `{"cmd":"go test"}`)

	if got["msgtype"] != "markdown" {
		t.Fatalf("msgtype = %v, want markdown", got["msgtype"])
	}
	markdown := got["markdown"].(map[string]interface{})
	if !strings.Contains(markdown["text"].(string), "bash") {
		t.Fatalf("markdown text = %q, want tool name", markdown["text"])
	}
}

func dingtalkSign(timestamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

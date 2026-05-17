package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/logger"
)

const conversationTypeGroup = "2"

type DingTalkBot struct {
	engine       *engine.AgentEngine
	clientSecret string
	httpClient   *http.Client
}

func NewDingTalkBot(eng *engine.AgentEngine) *DingTalkBot {
	return &DingTalkBot{
		engine:       eng,
		clientSecret: firstEnv("DINGTALK_CLIENT_SECRET", "DINGTALK_APP_SECRET"),
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

func NewDingTalkBotFromEnv(eng *engine.AgentEngine) (*DingTalkBot, error) {
	bot := NewDingTalkBot(eng)
	if bot.clientSecret == "" {
		return nil, fmt.Errorf("missing dingtalk credentials: DINGTALK_CLIENT_SECRET")
	}
	return bot, nil
}

func (b *DingTalkBot) Name() string {
	return "dingtalk"
}

func (b *DingTalkBot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := b.verifyCallback(r); err != nil {
		logger.WarnContext(r.Context(), "dingtalk callback verification failed", "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	var msg callbackMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		logger.WarnContext(r.Context(), "dingtalk callback parse failed", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	incoming := parseCallbackMessage(&msg)
	if incoming == nil || incoming.Content == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	ctx := logger.ContextWithAttrs(context.Background(),
		slog.String("platform", "dingtalk"),
		slog.String("conversation_id", incoming.ConversationID),
		slog.String("message_id", incoming.MessageID),
	)
	logger.InfoContext(ctx, "dingtalk message received", "content", incoming.Content)

	go b.handleAgentRun(ctx, incoming)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (b *DingTalkBot) verifyCallback(r *http.Request) error {
	if b.clientSecret == "" {
		return nil
	}

	timestamp := r.Header.Get("Timestamp")
	sign := r.Header.Get("Sign")
	if timestamp == "" || sign == "" {
		return fmt.Errorf("missing timestamp or sign header")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	diff := time.Now().UnixMilli() - ts
	if diff > int64(time.Hour/time.Millisecond) || diff < -int64(time.Hour/time.Millisecond) {
		return fmt.Errorf("timestamp expired")
	}

	stringToSign := timestamp + "\n" + b.clientSecret
	mac := hmac.New(sha256.New, []byte(b.clientSecret))
	_, _ = mac.Write([]byte(stringToSign))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sign), []byte(expected)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func (b *DingTalkBot) handleAgentRun(ctx context.Context, incoming *incomingMessage) {
	reporter := NewDingTalkReporter(incoming.SessionWebhook, b.httpClient)
	if err := b.engine.Run(ctx, incoming.Content, reporter); err != nil {
		logger.ErrorContext(ctx, "agent run failed", "err", err)
		reporter.OnMessage(ctx, fmt.Sprintf("Agent run failed: %v", err))
	}
}

type callbackMessage struct {
	ConversationID   string       `json:"conversationId"`
	ConversationType string       `json:"conversationType"`
	MsgID            string       `json:"msgId"`
	MsgType          string       `json:"msgtype"`
	Text             *textContent `json:"text"`
	SenderNick       string       `json:"senderNick"`
	SenderStaffID    string       `json:"senderStaffId"`
	SenderID         string       `json:"senderId"`
	SessionWebhook   string       `json:"sessionWebhook"`
	RobotCode        string       `json:"robotCode"`
	IsInAtList       bool         `json:"isInAtList"`
}

type textContent struct {
	Content string `json:"content"`
}

type incomingMessage struct {
	UserID         string
	UserName       string
	ConversationID string
	MessageID      string
	Content        string
	SessionWebhook string
}

func parseCallbackMessage(msg *callbackMessage) *incomingMessage {
	if msg == nil || msg.Text == nil {
		return nil
	}

	userID := msg.SenderStaffID
	if userID == "" {
		userID = msg.SenderID
	}
	conversationID := msg.ConversationID
	if msg.ConversationType != conversationTypeGroup {
		conversationID = userID
	}

	content := strings.TrimSpace(msg.Text.Content)
	if content == "" {
		return nil
	}

	return &incomingMessage{
		UserID:         userID,
		UserName:       msg.SenderNick,
		ConversationID: conversationID,
		MessageID:      msg.MsgID,
		Content:        content,
		SessionWebhook: msg.SessionWebhook,
	}
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

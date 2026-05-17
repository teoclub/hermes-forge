package wechat

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/logger"
)

const (
	ilinkBaseURL        = "https://ilinkai.weixin.qq.com"
	channelVersion      = "hermes-forge-0.1.0"
	longPollHTTPTimeout = 40 * time.Second
	reconnectBaseDelay  = time.Second
	reconnectMaxDelay   = 30 * time.Second
)

type WeChatBot struct {
	engine     *engine.AgentEngine
	botToken   string
	ilinkBotID string
	httpClient *http.Client
	cursor     string
}

func NewWeChatBotFromEnv(eng *engine.AgentEngine) (*WeChatBot, error) {
	botToken := os.Getenv("WECHAT_BOT_TOKEN")
	ilinkBotID := os.Getenv("WECHAT_ILINK_BOT_ID")
	if botToken == "" || ilinkBotID == "" {
		return nil, fmt.Errorf("missing wechat credentials: WECHAT_BOT_TOKEN/WECHAT_ILINK_BOT_ID")
	}
	return &WeChatBot{
		engine:     eng,
		botToken:   botToken,
		ilinkBotID: ilinkBotID,
		httpClient: &http.Client{Timeout: longPollHTTPTimeout},
	}, nil
}

func (b *WeChatBot) Name() string {
	return "wechat"
}

func (b *WeChatBot) Start(ctx context.Context, eng *engine.AgentEngine) error {
	if b.engine == nil {
		b.engine = eng
	}
	go func() {
		if err := b.pollLoop(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "wechat long poll stopped", "err", err)
		}
	}()
	return nil
}

func (b *WeChatBot) pollLoop(ctx context.Context) error {
	logger.InfoContext(ctx, "wechat long poll starting", "bot_id", b.ilinkBotID)
	attempts := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := b.pollOnce(ctx)
		if err == nil {
			attempts = 0
			continue
		}

		attempts++
		delay := pollReconnectDelay(attempts)
		logger.WarnContext(ctx, "wechat poll failed", "err", err, "delay", delay)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (b *WeChatBot) pollOnce(ctx context.Context) error {
	payload := map[string]interface{}{
		"get_updates_buf": b.cursor,
		"base_info":       newBaseInfo(),
	}

	var result getUpdatesResponse
	if err := b.ilinkPost(ctx, "/ilink/bot/getupdates", payload, &result); err != nil {
		return err
	}
	if result.Ret != 0 && result.ErrCode != 0 {
		return fmt.Errorf("getupdates error: ret=%d errcode=%d msg=%s", result.Ret, result.ErrCode, result.ErrMsg)
	}
	if result.GetUpdatesBuf != "" {
		b.cursor = result.GetUpdatesBuf
	}

	for i := range result.Msgs {
		incoming := parseMessage(&result.Msgs[i])
		if incoming == nil || incoming.Content == "" {
			continue
		}
		go b.handleAgentRun(ctx, incoming)
	}

	return nil
}

func (b *WeChatBot) handleAgentRun(parent context.Context, incoming *incomingMessage) {
	if parent == nil {
		parent = context.Background()
	}
	if parent.Err() != nil {
		return
	}

	ctx := logger.ContextWithAttrs(parent,
		slog.String("platform", "wechat"),
		slog.String("conversation_id", incoming.UserID),
		slog.String("message_id", incoming.MessageID),
	)

	logger.InfoContext(ctx, "wechat message received", "content", incoming.Content)
	reporter := NewWeChatReporter(b.botToken, incoming.UserID, incoming.ContextToken, b.httpClient)
	if err := b.engine.Run(ctx, incoming.Content, reporter); err != nil {
		logger.ErrorContext(ctx, "agent run failed", "err", err)
		reporter.OnMessage(ctx, fmt.Sprintf("Agent run failed: %v", err))
	}
}

func (b *WeChatBot) ilinkPost(ctx context.Context, path string, payload interface{}, out interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ilink payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ilinkBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create ilink request: %w", err)
	}
	setAuthHeaders(req, b.botToken, body)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ilink request %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read ilink response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ilink api %s returned status %d: %s", path, resp.StatusCode, string(respBody))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode ilink response: %w", err)
	}
	return nil
}

type incomingMessage struct {
	UserID       string
	Content      string
	MessageID    string
	ContextToken string
}

func parseMessage(msg *weChatMessage) *incomingMessage {
	if msg == nil || msg.MessageType == 2 || len(msg.ItemList) == 0 {
		return nil
	}

	item := msg.ItemList[0]
	content := ""
	switch item.Type {
	case 1:
		if item.TextItem != nil {
			content = strings.TrimSpace(item.TextItem.Text)
		}
	case 3:
		if item.VoiceItem != nil {
			content = strings.TrimSpace(item.VoiceItem.Text)
		}
	}
	if content == "" {
		return nil
	}

	return &incomingMessage{
		UserID:       msg.FromUserID,
		Content:      content,
		MessageID:    fmt.Sprintf("%d", msg.MessageID),
		ContextToken: msg.ContextToken,
	}
}

type baseInfo struct {
	ChannelVersion string `json:"channel_version"`
}

func newBaseInfo() baseInfo {
	return baseInfo{ChannelVersion: channelVersion}
}

type getUpdatesResponse struct {
	Ret           int             `json:"ret"`
	ErrCode       int             `json:"errcode"`
	ErrMsg        string          `json:"errmsg"`
	GetUpdatesBuf string          `json:"get_updates_buf"`
	Msgs          []weChatMessage `json:"msgs"`
}

type weChatMessage struct {
	MessageID    int64         `json:"message_id"`
	MessageType  int           `json:"message_type"`
	FromUserID   string        `json:"from_user_id"`
	ContextToken string        `json:"context_token"`
	ItemList     []messageItem `json:"item_list"`
}

type messageItem struct {
	Type      int        `json:"type"`
	TextItem  *textItem  `json:"text_item"`
	VoiceItem *voiceItem `json:"voice_item"`
}

type textItem struct {
	Text string `json:"text"`
}

type voiceItem struct {
	Text string `json:"text"`
}

func setAuthHeaders(req *http.Request, botToken string, body []byte) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+botToken)
	req.Header.Set("X-WECHAT-UIN", generateWeChatUIN())
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
}

func generateWeChatUIN() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	n := binary.BigEndian.Uint32(buf[:])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}

func pollReconnectDelay(attempt int) time.Duration {
	if attempt < 1 {
		return reconnectBaseDelay
	}
	shift := attempt - 1
	if shift > 30 {
		return reconnectMaxDelay
	}
	delay := reconnectBaseDelay * (1 << shift)
	if delay > reconnectMaxDelay {
		return reconnectMaxDelay
	}
	return delay
}

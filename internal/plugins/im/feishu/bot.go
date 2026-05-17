package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/logger"
)

const (
	connectionModeWebhook   = "webhook"
	connectionModeWebSocket = "websocket"
)

type FeishuBot struct {
	engine *engine.AgentEngine

	client    *lark.Client
	wsClient  *larkws.Client
	appID     string
	appSecret string

	mode string
}

func NewFeishuBot(eng *engine.AgentEngine) *FeishuBot {
	bot, err := NewFeishuBotFromEnv(eng)
	if err != nil {
		logger.Log(context.Background(), logger.LevelFatal, err.Error())
		os.Exit(1)
	}
	return bot
}

func NewFeishuBotFromEnv(eng *engine.AgentEngine) (*FeishuBot, error) {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")
	mode := resolveConnectionMode(os.Getenv("FEISHU_CONNECTION_MODE"))

	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("missing feishu credentials: FEISHU_APP_ID/FEISHU_APP_SECRET")
	}

	return &FeishuBot{
		client:    lark.NewClient(appID, appSecret),
		appID:     appID,
		appSecret: appSecret,
		engine:    eng,
		mode:      mode,
	}, nil
}

func (b *FeishuBot) Name() string {
	return "feishu"
}

func (b *FeishuBot) Start(ctx context.Context, eng *engine.AgentEngine) error {
	if b.engine == nil {
		b.engine = eng
	}

	switch b.mode {
	case connectionModeWebhook:
		return nil
	case connectionModeWebSocket:
		b.wsClient = larkws.NewClient(
			b.appID,
			b.appSecret,
			larkws.WithEventHandler(b.newEventDispatcher("", "")),
			larkws.WithAutoReconnect(true),
		)
		go func() {
			logger.InfoContext(ctx, "feishu websocket connecting", "app_id", b.appID)
			if err := b.wsClient.Start(ctx); err != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "feishu websocket stopped", "err", err)
			}
		}()
		return nil
	default:
		return fmt.Errorf("unsupported feishu connection mode: %s", b.mode)
	}
}

func (b *FeishuBot) ConnectionMode() string {
	return b.mode
}

func (b *FeishuBot) UsesWebhook() bool {
	return b.mode == connectionModeWebhook
}

func (b *FeishuBot) GetEventDispatcher() *larkdispatcher.EventDispatcher {
	encryptKey := os.Getenv("FEISHU_ENCRYPT_KEY")
	verifyToken := os.Getenv("FEISHU_VERIFY_TOKEN")
	return b.newEventDispatcher(verifyToken, encryptKey)
}

func (b *FeishuBot) newEventDispatcher(verifyToken, encryptKey string) *larkdispatcher.EventDispatcher {
	return larkdispatcher.NewEventDispatcher(verifyToken, encryptKey).
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			content, err := extractTextContent(event)
			if err != nil {
				logger.WarnContext(ctx, "feishu message parse failed", "err", err)
				return nil
			}

			chatID := *event.Event.Message.ChatId
			ctx = logger.ContextWithAttrs(ctx, slog.String("chat_id", chatID))
			logger.InfoContext(ctx, "feishu message received", "content", content)

			go b.handleAgentRun(ctx, chatID, content)
			return nil
		}).
		OnP2MessageReadV1(func(ctx context.Context, event *larkim.P2MessageReadV1) error {
			return nil
		})
}

func resolveConnectionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "ws", connectionModeWebSocket:
		return connectionModeWebSocket
	case "http", connectionModeWebhook:
		return connectionModeWebhook
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func (b *FeishuBot) handleAgentRun(ctx context.Context, chatID string, prompt string) {
	if ctx == nil {
		ctx = context.Background()
	}
	reporter := NewFeishuReporter(b.client, chatID)
	if err := b.engine.Run(ctx, prompt, reporter); err != nil {
		logger.ErrorContext(ctx, "agent run failed", "err", err)
		reporter.OnMessage(ctx, fmt.Sprintf("Agent run failed: %v", err))
	}
}

func extractTextContent(event *larkim.P2MessageReceiveV1) (string, error) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return "", fmt.Errorf("empty event")
	}
	if event.Event.Message.ChatId == nil || *event.Event.Message.ChatId == "" {
		return "", fmt.Errorf("missing chat_id")
	}
	if event.Event.Message.Content == nil {
		return "", fmt.Errorf("missing message.content")
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(*event.Event.Message.Content), &payload); err == nil && payload.Text != "" {
		return payload.Text, nil
	}

	content := *event.Event.Message.Content
	content = strings.TrimPrefix(content, `{"text":"`)
	content = strings.TrimSuffix(content, `"}`)
	if content == "" {
		return "", fmt.Errorf("empty text content")
	}
	return content, nil
}

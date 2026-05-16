package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	// larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/logger"
)

var _ engine.Reporter = (*FeishuReporter)(nil)

type FeishuBot struct {
	client    *lark.Client
	appID     string
	appSecret string
	engine    *engine.AgentEngine
}

func NewFeishuBot(eng *engine.AgentEngine) *FeishuBot {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")

	if appID == "" || appSecret == "" {
		logger.Log(context.Background(), logger.LevelFatal, "missing feishu credentials", "env", "FEISHU_APP_ID/FEISHU_APP_SECRET")
		os.Exit(1)
	}

	client := lark.NewClient(appID, appSecret)

	return &FeishuBot{
		client:    client,
		appID:     appID,
		appSecret: appSecret,
		engine:    eng,
	}
}

func (b *FeishuBot) GetEventDispatcher() *dispatcher.EventDispatcher {
	encryptKey := os.Getenv("FEISHU_ENCRYPT_KEY")
	verifyToken := os.Getenv("FEISHU_VERIFY_TOKEN")

	handler := dispatcher.NewEventDispatcher(verifyToken, encryptKey).
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			contentStr, err := extractTextContent(event)
			if err != nil {
				logger.WarnContext(ctx, "feishu message parse failed", "err", err)
				return nil
			}

			chatId := *event.Event.Message.ChatId
			ctx = logger.ContextWithAttrs(ctx, slog.String("chat_id", chatId))
			logger.InfoContext(ctx, "feishu message received", "content", contentStr)

			go b.handleAgentRun(chatId, contentStr)

			return nil
		}).
		OnP2MessageReadV1(func(ctx context.Context, event *larkim.P2MessageReadV1) error {
			// 消息已读事件，静默忽略
			return nil
		})

	return handler
}

func (b *FeishuBot) handleAgentRun(chatId string, prompt string) {
	ctx := logger.ContextWithAttrs(context.Background(), slog.String("chat_id", chatId))
	reporter := &FeishuReporter{
		client: b.client,
		chatId: chatId,
	}

	err := b.engine.Run(ctx, prompt, reporter)
	if err != nil {
		logger.ErrorContext(ctx, "agent run failed", "err", err)
		reporter.sendMsg(fmt.Sprintf("❌ Agent 运行崩溃: %v", err))
	}
}

type FeishuReporter struct {
	client *lark.Client
	chatId string
}

func (r *FeishuReporter) sendMsg(text string) {
	// Build text message content
	textContent := map[string]string{
		"text": text,
	}
	contentBytes, _ := json.Marshal(textContent)
	contentStr := string(contentBytes)

	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(r.chatId).
			MsgType(larkim.MsgTypeText).
			Content(contentStr).
			Build()).
		Build()

	resp, err := r.client.Im.Message.Create(context.Background(), msgReq)
	if err != nil {
		ctx := logger.ContextWithAttrs(context.Background(), slog.String("chat_id", r.chatId))
		logger.ErrorContext(ctx, "feishu message send failed", "err", err)
		return
	}
	if !resp.Success() {
		ctx := logger.ContextWithAttrs(context.Background(), slog.String("chat_id", r.chatId))
		logger.ErrorContext(ctx, "feishu message send failed", "code", resp.Code, "msg", resp.Msg)
	}
}

func (r *FeishuReporter) OnThinking(ctx context.Context) {
	r.sendMsg("🤔 模型正在慢思考 (Thinking)...")
}

func (r *FeishuReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.sendMsg(fmt.Sprintf("🛠️ **正在执行工具**：`%s`\n参数：`%s`", toolName, args))
}

func (r *FeishuReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.sendMsg(fmt.Sprintf("⚠️ **执行报错** (%s)：\n%s", toolName, result))
	} else {
		r.sendMsg(fmt.Sprintf("✅ **执行成功** (%s)", toolName))
	}
}

func (r *FeishuReporter) OnMessage(ctx context.Context, content string) {
	r.sendMsg(content)
}

func extractTextContent(event *larkim.P2MessageReceiveV1) (string, error) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return "", fmt.Errorf("空事件")
	}
	if event.Event.Message.ChatId == nil || *event.Event.Message.ChatId == "" {
		return "", fmt.Errorf("缺少 chat_id")
	}
	if event.Event.Message.Content == nil {
		return "", fmt.Errorf("缺少 message.content")
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(*event.Event.Message.Content), &payload); err == nil && payload.Text != "" {
		return payload.Text, nil
	}

	contentStr := *event.Event.Message.Content
	contentStr = strings.TrimPrefix(contentStr, `{"text":"`)
	contentStr = strings.TrimSuffix(contentStr, `"}`)
	if contentStr == "" {
		return "", fmt.Errorf("文本内容为空")
	}
	return contentStr, nil
}

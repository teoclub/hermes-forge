package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/logger"
)

var _ engine.Reporter = (*FeishuReporter)(nil)

type FeishuReporter struct {
	client *lark.Client
	chatID string
}

func NewFeishuReporter(client *lark.Client, chatID string) *FeishuReporter {
	return &FeishuReporter{client: client, chatID: chatID}
}

func (r *FeishuReporter) OnThinking(ctx context.Context) {
	r.sendMsg(ctx, "模型正在慢思考 (Thinking)...")
}

func (r *FeishuReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.sendMsg(ctx, fmt.Sprintf("**正在执行工具**：`%s`\n参数：`%s`", toolName, args))
}

func (r *FeishuReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.sendMsg(ctx, fmt.Sprintf("**执行报错** (%s)：\n%s", toolName, result))
		return
	}
	r.sendMsg(ctx, fmt.Sprintf("**执行成功** (%s)", toolName))
}

func (r *FeishuReporter) OnMessage(ctx context.Context, content string) {
	r.sendMsg(ctx, content)
}

func (r *FeishuReporter) sendMsg(ctx context.Context, text string) {
	if r == nil || r.client == nil || text == "" {
		return
	}
	textContent := map[string]string{
		"text": text,
	}
	contentBytes, err := json.Marshal(textContent)
	if err != nil {
		logger.ErrorContext(ctx, "feishu message marshal failed", "err", err)
		return
	}

	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(r.chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := r.client.Im.Message.Create(ctx, msgReq)
	if err != nil {
		logger.ErrorContext(ctx, "feishu message send failed", "err", err)
		return
	}
	if !resp.Success() {
		logger.ErrorContext(ctx, "feishu message send failed", "code", resp.Code, "msg", resp.Msg)
	}
}

package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/logger"
)

var _ engine.Reporter = (*WeChatReporter)(nil)

type WeChatReporter struct {
	botToken     string
	toUserID     string
	contextToken string
	httpClient   *http.Client
}

func NewWeChatReporter(botToken, toUserID, contextToken string, httpClient *http.Client) *WeChatReporter {
	return &WeChatReporter{
		botToken:     botToken,
		toUserID:     toUserID,
		contextToken: contextToken,
		httpClient:   httpClient,
	}
}

func (r *WeChatReporter) OnThinking(ctx context.Context) {
	r.sendText(ctx, "模型正在慢思考 (Thinking)...")
}

func (r *WeChatReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.sendText(ctx, fmt.Sprintf("正在执行工具：%s\n参数：%s", toolName, args))
}

func (r *WeChatReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.sendText(ctx, fmt.Sprintf("执行报错 (%s)：\n%s", toolName, result))
		return
	}
	r.sendText(ctx, fmt.Sprintf("执行成功 (%s)", toolName))
}

func (r *WeChatReporter) OnMessage(ctx context.Context, content string) {
	r.sendText(ctx, content)
}

func (r *WeChatReporter) sendText(ctx context.Context, text string) {
	if r == nil || r.botToken == "" || r.toUserID == "" || text == "" {
		return
	}

	payload := map[string]interface{}{
		"msg": map[string]interface{}{
			"from_user_id":  "",
			"to_user_id":    r.toUserID,
			"client_id":     fmt.Sprintf("hermes_forge_%d", time.Now().UnixNano()),
			"message_type":  2,
			"message_state": 2,
			"item_list": []map[string]interface{}{
				{
					"type":      1,
					"text_item": map[string]string{"text": text},
				},
			},
			"context_token": r.contextToken,
		},
		"base_info": newBaseInfo(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		logger.ErrorContext(ctx, "wechat message marshal failed", "err", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ilinkBaseURL+"/ilink/bot/sendmessage", bytes.NewReader(body))
	if err != nil {
		logger.ErrorContext(ctx, "wechat message request failed", "err", err)
		return
	}
	setAuthHeaders(req, r.botToken, body)

	client := r.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "wechat message send failed", "err", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		logger.ErrorContext(ctx, "wechat message send failed", "status", resp.StatusCode, "body", string(respBody))
	}
}

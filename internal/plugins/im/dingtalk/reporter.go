package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/logger"
)

var _ engine.Reporter = (*DingTalkReporter)(nil)

type DingTalkReporter struct {
	webhookURL string
	httpClient *http.Client
}

func NewDingTalkReporter(webhookURL string, httpClient *http.Client) *DingTalkReporter {
	return &DingTalkReporter{webhookURL: webhookURL, httpClient: httpClient}
}

func (r *DingTalkReporter) OnThinking(ctx context.Context) {
	r.sendMarkdown(ctx, "模型正在慢思考 (Thinking)...")
}

func (r *DingTalkReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.sendMarkdown(ctx, fmt.Sprintf("**正在执行工具**：`%s`\n\n参数：`%s`", toolName, args))
}

func (r *DingTalkReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.sendMarkdown(ctx, fmt.Sprintf("**执行报错** (%s)：\n\n%s", toolName, result))
		return
	}
	r.sendMarkdown(ctx, fmt.Sprintf("**执行成功** (%s)", toolName))
}

func (r *DingTalkReporter) OnMessage(ctx context.Context, content string) {
	r.sendMarkdown(ctx, content)
}

func (r *DingTalkReporter) sendMarkdown(ctx context.Context, text string) {
	if r == nil || r.webhookURL == "" || text == "" {
		return
	}
	client := r.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	body := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": "HermesForge",
			"text":  text,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		logger.ErrorContext(ctx, "dingtalk message marshal failed", "err", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.webhookURL, bytes.NewReader(payload))
	if err != nil {
		logger.ErrorContext(ctx, "dingtalk message request failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "dingtalk message send failed", "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logger.ErrorContext(ctx, "dingtalk message send failed", "status", resp.StatusCode, "body", string(respBody))
	}
}

package feishu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/teoclub/hermes-forge/internal/engine"
	pluginim "github.com/teoclub/hermes-forge/internal/plugins/im"
	"github.com/teoclub/hermes-forge/internal/schema"
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

var _ pluginim.FileDownloader = (*FeishuBot)(nil)

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
			incoming, err := extractIncomingMessage(event)
			if err != nil {
				logger.WarnContext(ctx, "feishu message parse failed", "err", err)
				return nil
			}
			if incoming == nil {
				return nil
			}

			ctx = logger.ContextWithAttrs(ctx,
				slog.String("chat_id", incoming.ChatID),
				slog.String("message_id", incoming.MessageID),
				slog.String("message_type", string(incoming.MessageType)),
			)
			logger.InfoContext(ctx, "feishu message received", "content", incoming.Content, "file_key", incoming.FileKey)

			go b.handleIncoming(context.WithoutCancel(ctx), incoming)
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

func (b *FeishuBot) handleIncoming(ctx context.Context, incoming *pluginim.IncomingMessage) {
	if ctx == nil {
		ctx = context.Background()
	}
	reporter := NewFeishuReporter(b.client, incoming.ChatID)

	switch incoming.MessageType {
	case pluginim.MessageTypeText:
		if err := b.engine.Run(ctx, incoming.Content, reporter); err != nil {
			logger.ErrorContext(ctx, "agent run failed", "err", err)
			reporter.OnMessage(ctx, fmt.Sprintf("Agent run failed: %v", err))
		}
	case pluginim.MessageTypeImage:
		image, err := b.downloadImageContent(ctx, incoming)
		if err != nil {
			logger.ErrorContext(ctx, "feishu image download failed", "err", err)
			reporter.OnMessage(ctx, fmt.Sprintf("图片下载失败: %v", err))
			return
		}
		prompt := incoming.Content
		if prompt == "" {
			prompt = "请分析这张图片。"
		}
		if err := b.engine.RunMessages(ctx, []schema.ContentPart{schema.Text(prompt), image}, reporter); err != nil {
			logger.ErrorContext(ctx, "agent image run failed", "err", err)
			reporter.OnMessage(ctx, fmt.Sprintf("Agent run failed: %v", err))
		}
	case pluginim.MessageTypeFile:
		name := incoming.FileName
		if name == "" {
			name = incoming.FileKey
		}
		reporter.OnMessage(ctx, fmt.Sprintf("已收到文件：%s。当前已支持下载该文件，但还没有接入文件内容分析流程。", name))
	}
}

func extractTextContent(event *larkim.P2MessageReceiveV1) (string, error) {
	incoming, err := extractIncomingMessage(event)
	if err != nil {
		return "", err
	}
	if incoming == nil || incoming.Content == "" {
		return "", fmt.Errorf("empty text content")
	}
	return incoming.Content, nil
}

func extractIncomingMessage(event *larkim.P2MessageReceiveV1) (*pluginim.IncomingMessage, error) {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil, fmt.Errorf("empty event")
	}
	msg := event.Event.Message
	if msg.ChatId == nil || *msg.ChatId == "" {
		return nil, fmt.Errorf("missing chat_id")
	}
	if msg.Content == nil {
		return nil, fmt.Errorf("missing message.content")
	}

	messageType := "text"
	if msg.MessageType != nil && *msg.MessageType != "" {
		messageType = *msg.MessageType
	}
	messageID := ""
	if msg.MessageId != nil {
		messageID = *msg.MessageId
	}
	threadID := messageID
	if msg.RootId != nil && *msg.RootId != "" {
		threadID = *msg.RootId
	}
	chatType := pluginim.ChatTypeDirect
	if msg.ChatType != nil && *msg.ChatType == "group" {
		chatType = pluginim.ChatTypeGroup
	}
	userID := ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil && event.Event.Sender.SenderId.OpenId != nil {
		userID = *event.Event.Sender.SenderId.OpenId
	}

	base := pluginim.IncomingMessage{
		Platform:  "feishu",
		UserID:    userID,
		ChatID:    *msg.ChatId,
		ChatType:  chatType,
		MessageID: messageID,
		ThreadID:  threadID,
	}

	switch messageType {
	case "text":
		content, err := parseFeishuTextContent(*msg.Content)
		if err != nil {
			return nil, err
		}
		base.MessageType = pluginim.MessageTypeText
		base.Content = stripFeishuMention(content, chatType)
		if base.Content == "" {
			return nil, nil
		}
		return &base, nil
	case "file":
		var payload struct {
			FileKey  string `json:"file_key"`
			FileName string `json:"file_name"`
		}
		if err := json.Unmarshal([]byte(*msg.Content), &payload); err != nil {
			return nil, fmt.Errorf("unmarshal file content: %w", err)
		}
		if payload.FileKey == "" {
			return nil, nil
		}
		base.MessageType = pluginim.MessageTypeFile
		base.FileKey = payload.FileKey
		base.FileName = payload.FileName
		return &base, nil
	case "image":
		var payload struct {
			ImageKey string `json:"image_key"`
		}
		if err := json.Unmarshal([]byte(*msg.Content), &payload); err != nil {
			return nil, fmt.Errorf("unmarshal image content: %w", err)
		}
		if payload.ImageKey == "" {
			return nil, nil
		}
		base.MessageType = pluginim.MessageTypeImage
		base.FileKey = payload.ImageKey
		base.FileName = payload.ImageKey + ".png"
		return &base, nil
	case "post":
		content, err := parseFeishuPostContent(*msg.Content)
		if err != nil {
			return nil, err
		}
		base.MessageType = pluginim.MessageTypeText
		base.Content = stripFeishuMention(content, chatType)
		if base.Content == "" {
			return nil, nil
		}
		return &base, nil
	default:
		return nil, nil
	}
}

func parseFeishuTextContent(content string) (string, error) {
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err == nil && payload.Text != "" {
		return payload.Text, nil
	}

	content = strings.TrimPrefix(content, `{"text":"`)
	content = strings.TrimSuffix(content, `"}`)
	if content == "" {
		return "", fmt.Errorf("empty text content")
	}
	return strings.TrimSpace(content), nil
}

func parseFeishuPostContent(content string) (string, error) {
	var payload struct {
		Title   string              `json:"title"`
		Content [][]json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return "", fmt.Errorf("unmarshal post content: %w", err)
	}

	var parts []string
	if payload.Title != "" {
		parts = append(parts, payload.Title)
	}
	for _, line := range payload.Content {
		var lineText strings.Builder
		for _, elem := range line {
			var tag struct {
				Tag  string `json:"tag"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(elem, &tag); err != nil {
				continue
			}
			switch tag.Tag {
			case "text", "a":
				lineText.WriteString(tag.Text)
			}
		}
		if text := strings.TrimSpace(lineText.String()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

func stripFeishuMention(content string, chatType pluginim.ChatType) string {
	content = strings.TrimSpace(content)
	if chatType != pluginim.ChatTypeGroup {
		return content
	}
	for strings.HasPrefix(content, "@_user_") {
		idx := strings.Index(content, " ")
		if idx < 0 {
			return ""
		}
		content = strings.TrimSpace(content[idx+1:])
	}
	return content
}

func (b *FeishuBot) DownloadFile(ctx context.Context, msg *pluginim.IncomingMessage) (io.ReadCloser, string, error) {
	if b == nil || b.client == nil {
		return nil, "", fmt.Errorf("missing feishu client")
	}
	if msg == nil || msg.FileKey == "" || msg.MessageID == "" {
		return nil, "", fmt.Errorf("file_key and message_id are required")
	}

	resourceType := "file"
	if msg.MessageType == pluginim.MessageTypeImage {
		resourceType = "image"
	}
	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(msg.MessageID).
		FileKey(msg.FileKey).
		Type(resourceType).
		Build()
	resp, err := b.client.Im.MessageResource.Get(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("download feishu resource: %w", err)
	}
	if resp == nil {
		return nil, "", fmt.Errorf("empty feishu resource response")
	}
	if !resp.Success() {
		return nil, "", fmt.Errorf("download feishu resource failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.File == nil {
		return nil, "", fmt.Errorf("empty feishu resource file")
	}
	fileName := msg.FileName
	if fileName == "" {
		fileName = resp.FileName
	}
	if fileName == "" {
		fileName = msg.FileKey
	}
	return io.NopCloser(resp.File), fileName, nil
}

func (b *FeishuBot) downloadImageContent(ctx context.Context, msg *pluginim.IncomingMessage) (*schema.ImageContent, error) {
	reader, fileName, err := b.DownloadFile(ctx, msg)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, pluginim.MaxDownloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if len(data) > pluginim.MaxDownloadBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", pluginim.MaxDownloadBytes)
	}
	mediaType := imageMediaType(fileName, data)
	return schema.ImageData(mediaType, base64.StdEncoding.EncodeToString(data)), nil
}

func imageMediaType(fileName string, data []byte) string {
	if ext := strings.ToLower(filepath.Ext(fileName)); ext != "" {
		if byExt := mime.TypeByExtension(ext); strings.HasPrefix(byExt, "image/") {
			return byExt
		}
	}
	if len(data) > 0 {
		if detected := http.DetectContentType(data); strings.HasPrefix(detected, "image/") {
			return detected
		}
	}
	return "image/png"
}

package wechat

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/teoclub/hermes-forge/internal/engine"
	pluginim "github.com/teoclub/hermes-forge/internal/plugins/im"
	"github.com/teoclub/hermes-forge/internal/schema"
	"github.com/teoclub/hermes-forge/logger"
)

const (
	ilinkBaseURL        = "https://ilinkai.weixin.qq.com"
	cdnBaseURL          = "https://novac2c.cdn.weixin.qq.com/c2c"
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

var _ pluginim.FileDownloader = (*WeChatBot)(nil)

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
		if incoming == nil {
			continue
		}
		go b.handleIncoming(context.WithoutCancel(ctx), incoming)
	}

	return nil
}

func (b *WeChatBot) handleIncoming(parent context.Context, incoming *incomingMessage) {
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
		slog.String("message_type", string(incoming.MessageType)),
	)

	logger.InfoContext(ctx, "wechat message received", "content", incoming.Content, "file_key", incoming.FileKey)
	reporter := NewWeChatReporter(b.botToken, incoming.UserID, incoming.ContextToken, b.httpClient)
	switch incoming.MessageType {
	case pluginim.MessageTypeText:
		if err := b.engine.Run(ctx, incoming.Content, reporter); err != nil {
			logger.ErrorContext(ctx, "agent run failed", "err", err)
			reporter.OnMessage(ctx, fmt.Sprintf("Agent run failed: %v", err))
		}
	case pluginim.MessageTypeImage:
		image, err := b.downloadImageContent(ctx, incoming.toPluginMessage())
		if err != nil {
			logger.ErrorContext(ctx, "wechat image download failed", "err", err)
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
	MessageType  pluginim.MessageType
	Content      string
	MessageID    string
	ContextToken string
	FileKey      string
	FileName     string
	FileSize     int64
	Extra        map[string]string
}

func (m *incomingMessage) toPluginMessage() *pluginim.IncomingMessage {
	if m == nil {
		return nil
	}
	extra := map[string]string{}
	for k, v := range m.Extra {
		extra[k] = v
	}
	if m.ContextToken != "" {
		extra["context_token"] = m.ContextToken
	}
	return &pluginim.IncomingMessage{
		Platform:    "wechat",
		MessageType: m.MessageType,
		UserID:      m.UserID,
		ChatType:    pluginim.ChatTypeDirect,
		Content:     m.Content,
		MessageID:   m.MessageID,
		FileKey:     m.FileKey,
		FileName:    m.FileName,
		FileSize:    m.FileSize,
		Extra:       extra,
	}
}

func parseMessage(msg *weChatMessage) *incomingMessage {
	if msg == nil || msg.MessageType == 2 || len(msg.ItemList) == 0 {
		return nil
	}

	item := msg.ItemList[0]
	switch item.Type {
	case 1:
		content := ""
		if item.TextItem != nil {
			content = strings.TrimSpace(item.TextItem.Text)
		}
		if content == "" {
			return nil
		}
		return &incomingMessage{
			UserID:       msg.FromUserID,
			MessageType:  pluginim.MessageTypeText,
			Content:      content,
			MessageID:    fmt.Sprintf("%d", msg.MessageID),
			ContextToken: msg.ContextToken,
		}
	case 2:
		if item.ImageItem == nil || item.ImageItem.Media == nil || item.ImageItem.Media.EncryptQueryParam == "" {
			return nil
		}
		aesKey := item.ImageItem.AESKey
		if aesKey == "" {
			aesKey = item.ImageItem.Media.AESKey
		}
		return &incomingMessage{
			UserID:       msg.FromUserID,
			MessageType:  pluginim.MessageTypeImage,
			MessageID:    fmt.Sprintf("%d", msg.MessageID),
			ContextToken: msg.ContextToken,
			FileKey:      BuildCDNDownloadURL(item.ImageItem.Media.EncryptQueryParam),
			FileName:     fmt.Sprintf("%d.png", msg.MessageID),
			Extra:        map[string]string{"aes_key": aesKey},
		}
	case 3:
		content := ""
		if item.VoiceItem != nil {
			content = strings.TrimSpace(item.VoiceItem.Text)
		}
		if content == "" {
			return nil
		}
		return &incomingMessage{
			UserID:       msg.FromUserID,
			MessageType:  pluginim.MessageTypeText,
			Content:      content,
			MessageID:    fmt.Sprintf("%d", msg.MessageID),
			ContextToken: msg.ContextToken,
		}
	case 4:
		if item.FileItem == nil || item.FileItem.Media == nil || item.FileItem.Media.EncryptQueryParam == "" {
			return nil
		}
		fileName := item.FileItem.FileName
		if fileName == "" {
			fileName = fmt.Sprintf("file_%d", msg.MessageID)
		}
		var fileSize int64
		if item.FileItem.Len != "" {
			_, _ = fmt.Sscanf(item.FileItem.Len, "%d", &fileSize)
		}
		return &incomingMessage{
			UserID:       msg.FromUserID,
			MessageType:  pluginim.MessageTypeFile,
			MessageID:    fmt.Sprintf("%d", msg.MessageID),
			ContextToken: msg.ContextToken,
			FileKey:      BuildCDNDownloadURL(item.FileItem.Media.EncryptQueryParam),
			FileName:     fileName,
			FileSize:     fileSize,
			Extra:        map[string]string{"aes_key": item.FileItem.Media.AESKey},
		}
	default:
		return nil
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
	ImageItem *imageItem `json:"image_item"`
	VoiceItem *voiceItem `json:"voice_item"`
	FileItem  *fileItem  `json:"file_item"`
}

type textItem struct {
	Text string `json:"text"`
}

type voiceItem struct {
	Media *cdnMedia `json:"media"`
	Text  string    `json:"text"`
}

type cdnMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param"`
	AESKey            string `json:"aes_key"`
}

type imageItem struct {
	Media  *cdnMedia `json:"media"`
	AESKey string    `json:"aeskey"`
	URL    string    `json:"url"`
}

type fileItem struct {
	Media    *cdnMedia `json:"media"`
	FileName string    `json:"file_name"`
	Len      string    `json:"len"`
}

func BuildCDNDownloadURL(encryptQueryParam string) string {
	return cdnBaseURL + "/download?encrypted_query_param=" + url.QueryEscape(encryptQueryParam)
}

func (b *WeChatBot) DownloadFile(ctx context.Context, msg *pluginim.IncomingMessage) (io.ReadCloser, string, error) {
	if msg == nil || msg.FileKey == "" {
		return nil, "", fmt.Errorf("no file URL in message")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, msg.FileKey, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create download request: %w", err)
	}
	client := b.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download file: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	fileName := msg.FileName
	if fileName == "" {
		fileName = msg.FileKey
	}
	aesKey := ""
	if msg.Extra != nil {
		aesKey = msg.Extra["aes_key"]
	}
	if aesKey == "" {
		return resp.Body, fileName, nil
	}

	encrypted, err := io.ReadAll(io.LimitReader(resp.Body, pluginim.MaxDownloadBytes+aes.BlockSize+1))
	resp.Body.Close()
	if err != nil {
		return nil, "", fmt.Errorf("read encrypted file: %w", err)
	}
	if len(encrypted) > pluginim.MaxDownloadBytes+aes.BlockSize {
		return nil, "", fmt.Errorf("download exceeds %d bytes", pluginim.MaxDownloadBytes)
	}
	key, err := parseAESKey(aesKey)
	if err != nil {
		return nil, "", fmt.Errorf("parse aes key: %w", err)
	}
	decrypted, err := decryptAES128ECB(encrypted, key)
	if err != nil {
		return nil, "", fmt.Errorf("decrypt file: %w", err)
	}
	return io.NopCloser(bytes.NewReader(decrypted)), fileName, nil
}

func (b *WeChatBot) downloadImageContent(ctx context.Context, msg *pluginim.IncomingMessage) (*schema.ImageContent, error) {
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
	return schema.ImageData(imageMediaType(fileName, data), base64.StdEncoding.EncodeToString(data)), nil
}

func parseAESKey(aesKeyStr string) ([]byte, error) {
	if aesKeyStr == "" {
		return nil, fmt.Errorf("empty aes key")
	}
	if len(aesKeyStr) == 32 && isHex(aesKeyStr) {
		return hex.DecodeString(aesKeyStr)
	}
	decoded, err := base64.StdEncoding.DecodeString(aesKeyStr)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(aesKeyStr)
		if err != nil {
			if isHex(aesKeyStr) && len(aesKeyStr)%2 == 0 {
				return hex.DecodeString(aesKeyStr)
			}
			return nil, fmt.Errorf("cannot decode aes key: %w", err)
		}
	}
	if len(decoded) == aes.BlockSize {
		return decoded, nil
	}
	if len(decoded) == 32 && isHex(string(decoded)) {
		return hex.DecodeString(string(decoded))
	}
	return nil, fmt.Errorf("aes key decoded to %d bytes, want 16 raw bytes or 32 hex chars", len(decoded))
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func decryptAES128ECB(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	bs := block.BlockSize()
	if len(ciphertext) == 0 || len(ciphertext)%bs != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size %d", len(ciphertext), bs)
	}
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += bs {
		block.Decrypt(plaintext[i:i+bs], ciphertext[i:i+bs])
	}
	if len(plaintext) > 0 {
		padLen := int(plaintext[len(plaintext)-1])
		if padLen > 0 && padLen <= bs && padLen <= len(plaintext) {
			valid := true
			for i := 0; i < padLen; i++ {
				if plaintext[len(plaintext)-1-i] != byte(padLen) {
					valid = false
					break
				}
			}
			if valid {
				plaintext = plaintext[:len(plaintext)-padLen]
			}
		}
	}
	return plaintext, nil
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

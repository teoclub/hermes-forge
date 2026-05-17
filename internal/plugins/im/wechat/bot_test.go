package wechat

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	pluginim "github.com/teoclub/hermes-forge/internal/plugins/im"
)

func TestParseMessageText(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:    42,
		MessageType:  1,
		FromUserID:   "user-1",
		ContextToken: "ctx-1",
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: " hello "},
			},
		},
	})

	if got == nil {
		t.Fatal("parseMessage returned nil")
	}
	if got.UserID != "user-1" {
		t.Fatalf("UserID = %q, want user-1", got.UserID)
	}
	if got.Content != "hello" {
		t.Fatalf("Content = %q, want hello", got.Content)
	}
	if got.MessageType != pluginim.MessageTypeText {
		t.Fatalf("MessageType = %q, want text", got.MessageType)
	}
	if got.MessageID != "42" {
		t.Fatalf("MessageID = %q, want 42", got.MessageID)
	}
	if got.ContextToken != "ctx-1" {
		t.Fatalf("ContextToken = %q, want ctx-1", got.ContextToken)
	}
}

func TestParseMessageVoiceText(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:   43,
		MessageType: 1,
		FromUserID:  "user-2",
		ItemList: []messageItem{
			{
				Type:      3,
				VoiceItem: &voiceItem{Text: "voice text"},
			},
		},
	})

	if got == nil {
		t.Fatal("parseMessage returned nil")
	}
	if got.Content != "voice text" {
		t.Fatalf("Content = %q, want voice text", got.Content)
	}
}

func TestParseMessageSkipsBotMessage(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:   44,
		MessageType: 2,
		FromUserID:  "bot",
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: "ignore me"},
			},
		},
	})

	if got != nil {
		t.Fatalf("parseMessage returned %#v, want nil", got)
	}
}

func TestParseMessageSkipsEmptyContent(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:   45,
		MessageType: 1,
		FromUserID:  "user-3",
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: "   "},
			},
		},
	})

	if got != nil {
		t.Fatalf("parseMessage returned %#v, want nil", got)
	}
}

func TestParseMessageImage(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:    46,
		MessageType:  1,
		FromUserID:   "user-4",
		ContextToken: "ctx-img",
		ItemList: []messageItem{
			{
				Type: 2,
				ImageItem: &imageItem{
					Media:  &cdnMedia{EncryptQueryParam: "abc+123", AESKey: "MTIzNDU2Nzg5MGFiY2RlZg=="},
					AESKey: "31323334353637383930616263646566",
				},
			},
		},
	})

	if got == nil {
		t.Fatal("parseMessage returned nil")
	}
	if got.MessageType != pluginim.MessageTypeImage {
		t.Fatalf("MessageType = %q, want image", got.MessageType)
	}
	if !strings.Contains(got.FileKey, "encrypted_query_param=abc%2B123") {
		t.Fatalf("FileKey = %q, want escaped CDN URL", got.FileKey)
	}
	if got.FileName != "46.png" {
		t.Fatalf("FileName = %q, want 46.png", got.FileName)
	}
	if got.Extra["aes_key"] != "31323334353637383930616263646566" {
		t.Fatalf("aes_key = %q, want image aeskey", got.Extra["aes_key"])
	}
	if got.ContextToken != "ctx-img" {
		t.Fatalf("ContextToken = %q, want ctx-img", got.ContextToken)
	}
}

func TestParseMessageFile(t *testing.T) {
	got := parseMessage(&weChatMessage{
		MessageID:   47,
		MessageType: 1,
		FromUserID:  "user-5",
		ItemList: []messageItem{
			{
				Type: 4,
				FileItem: &fileItem{
					Media:    &cdnMedia{EncryptQueryParam: "file-param", AESKey: "MTIzNDU2Nzg5MGFiY2RlZg=="},
					FileName: "doc.pdf",
					Len:      "1234",
				},
			},
		},
	})

	if got == nil {
		t.Fatal("parseMessage returned nil")
	}
	if got.MessageType != pluginim.MessageTypeFile {
		t.Fatalf("MessageType = %q, want file", got.MessageType)
	}
	if got.FileName != "doc.pdf" || got.FileSize != 1234 {
		t.Fatalf("file = (%q, %d), want (doc.pdf, 1234)", got.FileName, got.FileSize)
	}
	if !strings.Contains(got.FileKey, "encrypted_query_param=file-param") {
		t.Fatalf("FileKey = %q, want CDN URL", got.FileKey)
	}
}

func TestPollReconnectDelay(t *testing.T) {
	tests := map[int]time.Duration{
		0: reconnectBaseDelay,
		1: reconnectBaseDelay,
		2: 2 * reconnectBaseDelay,
		5: 16 * reconnectBaseDelay,
	}

	for attempt, want := range tests {
		if got := pollReconnectDelay(attempt); got != want {
			t.Fatalf("pollReconnectDelay(%d) = %s, want %s", attempt, got, want)
		}
	}

	if got := pollReconnectDelay(100); got != reconnectMaxDelay {
		t.Fatalf("pollReconnectDelay(100) = %s, want %s", got, reconnectMaxDelay)
	}
}

func TestParseAESKeyFormats(t *testing.T) {
	want := []byte("1234567890abcdef")
	tests := []string{
		"31323334353637383930616263646566",
		"MTIzNDU2Nzg5MGFiY2RlZg==",
		"MzEzMjMzMzQzNTM2MzczODM5MzA2MTYyNjM2NDY1NjY=",
	}
	for _, input := range tests {
		got, err := parseAESKey(input)
		if err != nil {
			t.Fatalf("parseAESKey(%q) returned error: %v", input, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("parseAESKey(%q) = %x, want %x", input, got, want)
		}
	}
}

func TestDecryptAES128ECB(t *testing.T) {
	key := []byte("1234567890abcdef")
	plaintext := []byte("hello")
	ciphertext := encryptECBForTest(t, plaintext, key)

	got, err := decryptAES128ECB(ciphertext, key)
	if err != nil {
		t.Fatalf("decryptAES128ECB returned error: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("plaintext = %q, want hello", got)
	}
}

func TestWeChatDownloadFileDecrypts(t *testing.T) {
	key := []byte("1234567890abcdef")
	ciphertext := encryptECBForTest(t, []byte("image-bytes"), key)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(ciphertext)),
			Header:     make(http.Header),
		}, nil
	})}
	bot := &WeChatBot{httpClient: client}

	reader, name, err := bot.DownloadFile(context.Background(), &pluginim.IncomingMessage{
		MessageType: pluginim.MessageTypeImage,
		FileKey:     "https://example.test/download",
		FileName:    "img.png",
		Extra:       map[string]string{"aes_key": "MTIzNDU2Nzg5MGFiY2RlZg=="},
	})
	if err != nil {
		t.Fatalf("DownloadFile returned error: %v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read downloaded data: %v", err)
	}
	if name != "img.png" || string(data) != "image-bytes" {
		t.Fatalf("download = (%q, %q), want (img.png, image-bytes)", name, data)
	}
}

func encryptECBForTest(t *testing.T, plaintext, key []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	bs := block.BlockSize()
	padLen := bs - len(plaintext)%bs
	padded := append([]byte(nil), plaintext...)
	for i := 0; i < padLen; i++ {
		padded = append(padded, byte(padLen))
	}
	ciphertext := make([]byte, len(padded))
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(ciphertext[i:i+bs], padded[i:i+bs])
	}
	return ciphertext
}

func TestWeChatReporterSendsText(t *testing.T) {
	var captured requestCapture
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured.method = req.Method
		captured.path = req.URL.Path
		captured.authType = req.Header.Get("AuthorizationType")
		captured.auth = req.Header.Get("Authorization")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &captured.body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"ret":0}`))),
			Header:     make(http.Header),
		}, nil
	})}

	reporter := NewWeChatReporter("token-1", "user-1", "ctx-1", client)
	reporter.OnMessage(context.Background(), "hello")

	if captured.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", captured.method)
	}
	if captured.path != "/ilink/bot/sendmessage" {
		t.Fatalf("path = %s, want /ilink/bot/sendmessage", captured.path)
	}
	if captured.authType != "ilink_bot_token" {
		t.Fatalf("AuthorizationType = %q, want ilink_bot_token", captured.authType)
	}
	if captured.auth != "Bearer token-1" {
		t.Fatalf("Authorization = %q, want Bearer token-1", captured.auth)
	}

	msg := captured.body["msg"].(map[string]interface{})
	if msg["to_user_id"] != "user-1" {
		t.Fatalf("to_user_id = %v, want user-1", msg["to_user_id"])
	}
	if msg["context_token"] != "ctx-1" {
		t.Fatalf("context_token = %v, want ctx-1", msg["context_token"])
	}
	items := msg["item_list"].([]interface{})
	textItem := items[0].(map[string]interface{})["text_item"].(map[string]interface{})
	if text := textItem["text"].(string); !strings.Contains(text, "hello") {
		t.Fatalf("text = %q, want hello", text)
	}
}

type requestCapture struct {
	method   string
	path     string
	authType string
	auth     string
	body     map[string]interface{}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

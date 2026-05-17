package im

import (
	"context"
	"io"
)

type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeImage MessageType = "image"
	MessageTypeFile  MessageType = "file"
)

const MaxDownloadBytes = 20 << 20

type ChatType string

const (
	ChatTypeDirect ChatType = "direct"
	ChatTypeGroup  ChatType = "group"
)

type IncomingMessage struct {
	Platform    string
	MessageType MessageType
	UserID      string
	ChatID      string
	ChatType    ChatType
	Content     string
	MessageID   string
	ThreadID    string
	FileKey     string
	FileName    string
	FileSize    int64
	Extra       map[string]string
}

type FileDownloader interface {
	DownloadFile(ctx context.Context, msg *IncomingMessage) (io.ReadCloser, string, error)
}

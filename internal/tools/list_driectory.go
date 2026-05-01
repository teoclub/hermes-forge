package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ Tool = (*ListDirectoryTool)(nil)

type ListDirectoryTool struct {
	workDir string
}

// Definition implements [Tool].
func (l *ListDirectoryTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        l.Name(),
		Description: "列出指定目录下的文件和子目录。请提供相对于工作区的目录路径；不提供 path 时默认列出工作区根目录。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "要列出的目录路径，如 internal/tools；为空时表示当前工作区根目录",
				},
			},
		},
	}
}

// Execute implements [Tool].
func (l *ListDirectoryTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input listDirectoryArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
	}

	relPath := strings.TrimSpace(input.Path)
	if relPath == "" {
		relPath = "."
	}

	fullPath := filepath.Join(l.workDir, relPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取目录信息失败: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("路径不是目录: %s", relPath)
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取目录失败: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Sprintf("目录为空: %s", relPath), nil
	}

	const maxEntries = 200
	var b strings.Builder
	fmt.Fprintf(&b, "目录: %s\n", relPath)

	limit := len(entries)
	truncated := false
	if limit > maxEntries {
		limit = maxEntries
		truncated = true
	}

	for i := 0; i < limit; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		entry := entries[i]
		name := entry.Name()
		if entry.IsDir() {
			fmt.Fprintf(&b, "- [dir] %s/\n", name)
			continue
		}

		entryInfo, err := entry.Info()
		if err != nil {
			fmt.Fprintf(&b, "- [file] %s (无法读取信息: %v)\n", name, err)
			continue
		}
		fmt.Fprintf(&b, "- [file] %s (%s)\n", name, formatFileSize(entryInfo.Size()))
	}

	if truncated {
		fmt.Fprintf(&b, "\n...[目录项过多，仅显示前 %d 项，共 %d 项]...", maxEntries, len(entries))
	}

	return b.String(), nil
}

// Name implements [Tool].
func (l *ListDirectoryTool) Name() string {
	return "list_directory"
}

func NewListDirectoryTool(workDir string) *ListDirectoryTool {
	return &ListDirectoryTool{workDir: workDir}
}

type listDirectoryArgs struct {
	Path string `json:"path"`
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}

	return fmt.Sprintf("%.1f PB", value/unit)
}

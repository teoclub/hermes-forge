package tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ Tool = (*ReadFileTool)(nil)

type ReadFileTool struct {
	workDir string
}

func NewReadFileTool(workDir string) *ReadFileTool {
	return &ReadFileTool{workDir: workDir}
}

func (r *ReadFileTool) Definition() schema.ToolDefinition {
	slog.Info("Initializing ReadFileTool")
	return schema.ToolDefinition{
		Name:        r.Name(),
		Description: "读取文件内容",
		InputSchema: map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The path to the file to read.",
				"required":    true,
			},
		},
	}
}

func (r *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	slog.Info("Executing ReadFileTool")
	return "", nil
}

func (r *ReadFileTool) Name() string {
	return "read_file"
}

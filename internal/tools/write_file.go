package tools

import (
	"context"
	"encoding/json"

	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ Tool = (*WriteFileTool)(nil)

type WriteFileTool struct {
	workDir string
}

func NewWriteFileTool(workDir string) *WriteFileTool {
	return &WriteFileTool{workDir: workDir}
}

// Definition implements [Tool].
func (w *WriteFileTool) Definition() schema.ToolDefinition {
	panic("unimplemented")
}

// Execute implements [Tool].
func (w *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	panic("unimplemented")
}

// Name implements [Tool].
func (w *WriteFileTool) Name() string {
	panic("unimplemented")
}

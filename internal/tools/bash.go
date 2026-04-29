package tools

import (
	"context"
	"encoding/json"

	"github.com/teoclub/hermes-forge/internal/schema"
)

var _ Tool = (*BashTool)(nil)

type BashTool struct {
	workDir string
}

func NewBashTool(workDir string) *BashTool {
	return &BashTool{workDir: workDir}
}

// Definition implements [Tool].
func (t *BashTool) Definition() schema.ToolDefinition {
	panic("unimplemented")
}

// Execute implements [Tool].
func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	panic("unimplemented")
}

func (t *BashTool) Name() string {
	return "bash"
}

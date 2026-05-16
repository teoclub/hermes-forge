package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/teoclub/hermes-forge/internal/schema"
	"github.com/teoclub/hermes-forge/logger"
)

type Tool interface {
	Name() string
	Definition() schema.ToolDefinition
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

type Registry interface {
	Register(tool Tool)
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult
	GetAvailableTools() []schema.ToolDefinition
}

type registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func (r *registry) GetAvailableTools() []schema.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var defs []schema.ToolDefinition
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

func NewRegistry() Registry {
	return &registry{
		tools: make(map[string]Tool),
	}
}

func (r *registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		logger.Warn("tool already registered", "name", name)
	}
	r.tools[name] = tool

	logger.Info("tool registered", "name", name)
}

func (r *registry) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	r.mu.RLock()
	tool, exists := r.tools[call.Name]
	r.mu.RUnlock()
	if !exists {
		errMsg := fmt.Sprintf("Error: 系统中不存在名为 '%s' 的工具。", call.Name)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     fmt.Sprintf("Error: 执行工具 '%s' 时发生错误: %v", call.Name, err),
			IsError:    true,
		}
	}
	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     result,
		IsError:    false,
	}
}

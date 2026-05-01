package feishu

import (
	"context"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/internal/plugin"
)

var _ engine.Reporter = (*FeishuReporter)(nil)
var _ plugin.Plugin = (*FeishuBot)(nil)

type FeishuBot struct {
	engine *engine.AgentEngine
}

// Name implements [plugin.Plugin].
func (f *FeishuBot) Name() string {
	panic("unimplemented")
}

// Start implements [plugin.Plugin].
func (f *FeishuBot) Start(ctx context.Context, eng *engine.AgentEngine) error {
	panic("unimplemented")
}

func NewFeishuBot(engine *engine.AgentEngine) *FeishuBot {
	return &FeishuBot{
		engine: engine,
	}
}

type FeishuReporter struct {
}

// OnMessage implements [engine.Reporter].
func (f *FeishuReporter) OnMessage(ctx context.Context, content string) {
	panic("unimplemented")
}

// OnThinking implements [engine.Reporter].
func (f *FeishuReporter) OnThinking(ctx context.Context) {
	panic("unimplemented")
}

// OnToolCall implements [engine.Reporter].
func (f *FeishuReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	panic("unimplemented")
}

// OnToolResult implements [engine.Reporter].
func (f *FeishuReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	panic("unimplemented")
}

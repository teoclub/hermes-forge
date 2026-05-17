package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
	"github.com/teoclub/hermes-forge/internal/tools"
	"github.com/teoclub/hermes-forge/logger"
)

type AgentEngine struct {
	provider       provider.LLMProvider
	registry       tools.Registry
	workDir        string
	enableThinking bool
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string, enableThinking bool) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		workDir:        workDir,
		enableThinking: enableThinking,
	}
}

func (e *AgentEngine) Run(ctx context.Context, userPrompt string, reporter Reporter) error {
	return e.RunMessages(ctx, []schema.ContentPart{schema.Text(userPrompt)}, reporter)
}

func (e *AgentEngine) RunMessages(ctx context.Context, userParts []schema.ContentPart, reporter Reporter) error {
	logger.InfoContext(ctx, "engine started", "work_dir", e.workDir, "thinking", e.enableThinking)
	if len(userParts) == 0 {
		userParts = []schema.ContentPart{schema.Text("")}
	}

	contextHistory := []schema.Message{
		{Role: schema.RoleSystem, Content: []schema.ContentPart{schema.Text("You are hermes-forge, an expert coding assistant.")}},
		{Role: schema.RoleUser, Content: append([]schema.ContentPart(nil), userParts...)},
	}

	turnCount := 0

	for {
		turnCount++
		logger.InfoContext(ctx, "engine turn started", "turn", turnCount)

		availableTools := e.registry.GetAvailableTools()

		// Phase 1: 慢思考阶段
		if e.enableThinking {
			if reporter != nil {
				reporter.OnThinking(ctx)
			}

			logger.InfoContext(ctx, "thinking phase started", "turn", turnCount)
			thinkResp, err := e.provider.Generate(ctx, contextHistory, nil)
			if err != nil {
				return fmt.Errorf("Thinking 阶段失败: %w", err)
			}
			if content := schema.MessageText(thinkResp.Message.Content); content != "" {
				logger.DebugContext(ctx, "thinking phase response", "turn", turnCount, "content", content)
				contextHistory = append(contextHistory, thinkResp.Message)
			}
		}

		// Phase 2: 行动阶段
		logger.InfoContext(ctx, "action phase started", "turn", turnCount, "tools", len(availableTools))
		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("Action 阶段失败: %w", err)
		}

		contextHistory = append(contextHistory, actionResp.Message)

		if len(actionResp.Message.Content) > 0 {
			logger.InfoContext(ctx, "model responded", "turn", turnCount, "content", schema.MessageText(actionResp.Message.Content))
		} else {
			logger.InfoContext(ctx, "model responded without text", "turn", turnCount, "tool_calls", len(actionResp.Message.ToolCalls))
		}

		if reporter != nil {
			content := schema.MessageText(actionResp.Message.Content)
			if content != "" {
				reporter.OnMessage(ctx, content)
			}
		}

		if len(actionResp.Message.ToolCalls) == 0 {
			logger.InfoContext(ctx, "engine completed", "turn", turnCount)
			break
		}

		logger.InfoContext(ctx, "tool calls requested", "turn", turnCount, "count", len(actionResp.Message.ToolCalls))

		// 预分配切片以保证顺序并避免并发写入锁
		observationMsgs := make([]schema.Message, len(actionResp.Message.ToolCalls))
		var wg sync.WaitGroup

		for i, toolCall := range actionResp.Message.ToolCalls {
			wg.Add(1)

			go func(idx int, call schema.ToolCall) {
				defer wg.Done()

				logger.InfoContext(ctx, "tool call started", "turn", turnCount, "index", idx, "tool", call.Name)

				if reporter != nil {
					reporter.OnToolCall(ctx, call.Name, string(call.Arguments))
				}

				// 执行底层工具并获取结果
				result := e.registry.Execute(ctx, call)

				if result.IsError {
					logger.ErrorContext(ctx, "tool call failed", "turn", turnCount, "index", idx, "tool", call.Name, "output", result.Output)
				} else {
					logger.InfoContext(ctx, "tool call succeeded", "turn", turnCount, "index", idx, "tool", call.Name, "output_bytes", len(result.Output))
				}

				if reporter != nil {
					displayOutput := result.Output
					if len(displayOutput) > 200 {
						displayOutput = displayOutput[:200] + "... (已截断)"
					}
					reporter.OnToolResult(ctx, call.Name, displayOutput, result.IsError)
				}

				// 安全写入对应索引
				observationMsgs[idx] = schema.Message{
					Role:       schema.RoleTool,
					Content:    []schema.ContentPart{schema.Text(result.Output)},
					ToolCallID: call.ID,
				}
			}(i, toolCall)
		}

		// 等待所有工具调用完成
		wg.Wait()

		logger.InfoContext(ctx, "tool calls completed", "turn", turnCount, "count", len(observationMsgs))

		// 按序追加回 Context
		for _, obs := range observationMsgs {
			contextHistory = append(contextHistory, obs)
		}
	}

	return nil
}

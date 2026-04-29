package engine

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
	"github.com/teoclub/hermes-forge/internal/tools"
)

type AgentEngine struct {
	provider       provider.LLMProvider
	registry       tools.Registry
	WorkDir        string
	EnableThinking bool
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string, enableThinking bool) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		WorkDir:        workDir,
		EnableThinking: enableThinking,
	}
}

func (e *AgentEngine) Run(ctx context.Context, userPrompt string, reporter Reporter) error {
	log.Printf("[Engine] 引擎启动，锁定工作区: %s\n", e.WorkDir)

	contextHistory := []schema.Message{
		{Role: schema.RoleSystem, Content: []schema.ContentPart{schema.Text("You are go-tiny-claw, an expert coding assistant.")}},
		{Role: schema.RoleUser, Content: []schema.ContentPart{schema.Text(userPrompt)}},
	}

	for {
		availableTools := e.registry.GetAvailableTools()

		if e.EnableThinking {
			if reporter != nil {
				reporter.OnThinking(ctx)
			}

			thinkResp, err := e.provider.Generate(ctx, contextHistory, nil)
			if err != nil {
				return fmt.Errorf("Thinking 阶段失败: %w", err)
			}
			if schema.MessageText(thinkResp.Content) != "" {
				contextHistory = append(contextHistory, *thinkResp)
			}
		}

		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("Action 阶段失败: %w", err)
		}

		contextHistory = append(contextHistory, *actionResp)

		if reporter != nil {
			content := schema.MessageText(actionResp.Content)
			if content != "" {
				reporter.OnMessage(ctx, content)
			}
		}

		if len(actionResp.ToolCalls) == 0 {
			break
		}

		observationMsgs := make([]schema.Message, len(actionResp.ToolCalls))
		var wg sync.WaitGroup

		for i, toolCall := range actionResp.ToolCalls {
			wg.Add(1)

			go func(idx int, call schema.ToolCall) {
				defer wg.Done()

				if reporter != nil {
					reporter.OnToolCall(ctx, call.Name, string(call.Arguments))
				}

				result := e.registry.Execute(ctx, call)

				if reporter != nil {
					displayOutput := result.Output
					if len(displayOutput) > 200 {
						displayOutput = displayOutput[:200] + "... (已截断)"
					}
					reporter.OnToolResult(ctx, call.Name, displayOutput, result.IsError)
				}

				observationMsgs[idx] = schema.Message{
					Role:       schema.RoleUser,
					Content:    []schema.ContentPart{schema.Text(result.Output)},
					ToolCallID: call.ID,
				}
			}(i, toolCall)
		}

		wg.Wait()

		for _, obs := range observationMsgs {
			contextHistory = append(contextHistory, obs)
		}
	}

	return nil
}

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
	log.Printf("[Engine] 引擎启动，锁定工作区: %s\n", e.workDir)
	log.Printf("[Engine] 慢思考模式 (Thinking Phase): %v\n", e.enableThinking)

	contextHistory := []schema.Message{
		{Role: schema.RoleSystem, Content: []schema.ContentPart{schema.Text("You are hermes-forge, an expert coding assistant.")}},
		{Role: schema.RoleUser, Content: []schema.ContentPart{schema.Text(userPrompt)}},
	}

	turnCount := 0

	for {
		turnCount++
		log.Printf("\n========== [Turn %d] 开始 ==========\n", turnCount)

		availableTools := e.registry.GetAvailableTools()

		// Phase 1: 慢思考阶段
		if e.enableThinking {
			if reporter != nil {
				reporter.OnThinking(ctx)
			}

			log.Println("[Engine][Phase 1] 剥夺工具访问权，强制进入慢思考与规划阶段...")
			thinkResp, err := e.provider.Generate(ctx, contextHistory, nil)
			if err != nil {
				return fmt.Errorf("Thinking 阶段失败: %w", err)
			}
			if schema.MessageText(thinkResp.Message.Content) != "" {
				fmt.Printf("🧠 [内部思考 Trace]: \n%s\n", thinkResp.Message)
				contextHistory = append(contextHistory, thinkResp.Message)
			}
		}

		// Phase 2: 行动阶段
		log.Println("[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...")
		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("Action 阶段失败: %w", err)
		}

		contextHistory = append(contextHistory, actionResp.Message)

		if len(actionResp.Message.Content) > 0 {
			log.Printf("🤖 [模型回复]: \n%s\n", actionResp.Message)
		} else {
			log.Printf("🤖 [模型回复]: (无文本内容，可能仅包含工具调用)\n")
		}

		if reporter != nil {
			content := schema.MessageText(actionResp.Message.Content)
			if content != "" {
				reporter.OnMessage(ctx, content)
			}
		}

		if len(actionResp.Message.ToolCalls) == 0 {
			log.Println("[Engine] 模型未请求调用工具，任务宣告完成。")
			break
		}

		log.Printf("[Engine] 模型请求并发调用 %d 个工具...\n", len(actionResp.Message.ToolCalls))

		// 预分配切片以保证顺序并避免并发写入锁
		observationMsgs := make([]schema.Message, len(actionResp.Message.ToolCalls))
		var wg sync.WaitGroup

		for i, toolCall := range actionResp.Message.ToolCalls {
			wg.Add(1)

			go func(idx int, call schema.ToolCall) {
				defer wg.Done()

				log.Printf("  -> [Go-%d] 🛠️ 触发并行执行: %s\n", idx, call.Name)

				if reporter != nil {
					reporter.OnToolCall(ctx, call.Name, string(call.Arguments))
				}

				// 执行底层工具并获取结果
				result := e.registry.Execute(ctx, call)

				if result.IsError {
					log.Printf("  -> [Go-%d] ❌ 工具执行报错: %s\n", idx, result.Output)
				} else {
					log.Printf("  -> [Go-%d] ✅ 工具执行成功 (返回 %d 字节)\n", idx, len(result.Output))
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
					Role:       schema.RoleUser,
					Content:    []schema.ContentPart{schema.Text(result.Output)},
					ToolCallID: call.ID,
				}
			}(i, toolCall)
		}

		// 等待所有工具调用完成
		wg.Wait()

		log.Println("[Engine] 所有并发工具执行完毕，开始聚合观察结果 (Observation)...")

		// 按序追加回 Context
		for _, obs := range observationMsgs {
			contextHistory = append(contextHistory, obs)
		}
	}

	return nil
}

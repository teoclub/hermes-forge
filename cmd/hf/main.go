package main

import (
	"context"
	"log"
	"os"

	"github.com/teoclub/hermes-forge/internal/client"
	"github.com/teoclub/hermes-forge/internal/engine"

	"github.com/teoclub/hermes-forge/internal/provider/anthropic"
	"github.com/teoclub/hermes-forge/internal/tools"
)

func main() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		log.Fatal("请先导出 ANTHROPIC_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()

	llmProvider, err := anthropic.NewAnthropicProvider(
		client.WithModel("claude-3-5-sonnet-latest"),
	)
	if err != nil {
		log.Fatal(err)
	}
	registry := tools.NewRegistry()

	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))

	// 开启慢思考，促使大模型一次性规划出并行的工具调用
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, true)

	prompt := `
	我当前目录下有 a.txt, b.txt, c.txt 三个文件。(如果没有请忽略找不到的报错)
	为了节省时间，请你同时一次性利用工具读取这三个文件，并将它们的内容综合起来告诉我。
	`

	err = eng.Run(context.Background(), prompt, nil)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}

package main

import (
	"context"
	"log"
	"os"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/tools"

	_ "github.com/teoclub/hermes-forge/internal/provider/anthropic"
)

func main() {
	workDir, _ := os.Getwd()
	modelName := os.Getenv("HF_MODEL")
	if modelName == "" {
		log.Fatal("请先导出 HF_MODEL 环境变量")
	}
	apiKey := os.Getenv("HF_API_KEY")
	if apiKey == "" {
		log.Fatal("请先导出 HF_API_KEY 环境变量")
	}
	baseURL := os.Getenv("HF_BASE_URL")
	if baseURL == "" {
		log.Fatal("请先导出 HF_BASE_URL 环境变量")
	}
	llmProvider, err := provider.New(
		"anthropic",
		provider.WithAPIKey(apiKey),
		provider.WithBaseURL(baseURL),
		provider.WithModel(modelName),
		provider.WithMaxTokens(128),
	)

	registry := tools.NewRegistry()

	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 开启慢思考，促使大模型一次性规划出并行的工具调用
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, true)

	prompt := `
	请帮我执行以下操作： 
	1. 用 bash 查看一下我当前电脑的 Go 版本。 
	2. 帮我写一个简单的 helloworld.go 文件，输出 "Hello, hermes forge!"。 
	3. 用 bash 编译并运行这个 go 文件，确认它能正常工作。
	`

	err = eng.Run(context.Background(), prompt, nil)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}

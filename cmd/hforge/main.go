package main

import (
	"context"
	"log"
	"os"

	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/tools"
)

func main() {
	workDir, _ := os.Getwd()
	providerName := os.Getenv("HF_PROVIDER")
	modelName := os.Getenv("HF_MODEL")
	apiKey := os.Getenv("HF_API_KEY")

	llmProvider, err := provider.New(
		providerName,
		provider.WithAPIKey(apiKey),
		provider.WithModel(modelName),
	)
	if err != nil {
		log.Fatalf("初始化 provider 失败: %v (available=%v)", err, provider.Registered())
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

	if err := eng.Run(context.Background(), prompt, nil); err != nil {
		log.Fatalf("执行失败: %v", err)
	}
}

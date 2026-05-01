package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/internal/plugin/feishu"
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/tools"

	_ "github.com/teoclub/hermes-forge/internal/provider/anthropic"
	_ "github.com/teoclub/hermes-forge/internal/provider/openai"
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

	if err != nil {
		log.Fatalf("初始化 provider 失败: %v (available=%v)", err, provider.RegisteredProviders())
	}
	registry := tools.NewRegistry()

	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 开启慢思考，促使大模型一次性规划出并行的工具调用
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, true)

	bot := feishu.NewFeishuBot(eng)
	handler := httpserverext.NewEventHandlerFunc(bot.GetEventDispatcher())

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/webhook/event", logHTTP(handler))

	port := ":48080"
	log.Printf("Provider 已初始化: name=%s model=%s base_url=%s", llmProvider.Name(), modelName, baseURL)
	log.Printf("🚀 hermes-forge 飞书服务端已启动，正在监听 %s 端口\n", port)

	err = http.ListenAndServe(port, mux)
	if err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

func logHTTP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		log.Printf("[HTTP] %s %s status=%d duration=%s remote=%s", r.Method, r.URL.Path, rec.status, time.Since(start), r.RemoteAddr)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

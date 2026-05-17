package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	"github.com/teoclub/hermes-forge/internal/engine"
	"github.com/teoclub/hermes-forge/internal/plugins/im/dingtalk"
	"github.com/teoclub/hermes-forge/internal/plugins/im/feishu"
	"github.com/teoclub/hermes-forge/internal/plugins/im/wechat"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})

	if bot, err := feishu.NewFeishuBotFromEnv(eng); err == nil {
		if err := bot.Start(ctx, eng); err != nil {
			log.Printf("Feishu plugin start failed: %v", err)
		} else if bot.UsesWebhook() {
			handler := httpserverext.NewEventHandlerFunc(bot.GetEventDispatcher())
			mux.HandleFunc("/webhook/feishu/event", logHTTP(handler))
			log.Printf("Feishu plugin enabled: webhook /webhook/feishu/event")
		} else {
			log.Printf("Feishu plugin enabled: %s", bot.ConnectionMode())
		}
	} else {
		log.Printf("Feishu plugin disabled: %v", err)
	}

	if dingBot, err := dingtalk.NewDingTalkBotFromEnv(eng); err == nil {
		mux.HandleFunc("/webhook/dingtalk/event", logHTTP(dingBot.ServeHTTP))
		log.Printf("DingTalk plugin enabled: /webhook/dingtalk/event")
	} else {
		log.Printf("DingTalk plugin disabled: %v", err)
	}

	if wechatBot, err := wechat.NewWeChatBotFromEnv(eng); err == nil {
		if err := wechatBot.Start(ctx, eng); err != nil {
			log.Printf("WeChat plugin start failed: %v", err)
		} else {
			log.Printf("WeChat plugin enabled: long-poll")
		}
	} else {
		log.Printf("WeChat plugin disabled: %v", err)
	}

	port := ":48080"
	log.Printf("Provider 已初始化: name=%s model=%s base_url=%s", llmProvider.Name(), modelName, baseURL)
	log.Printf("hermes-forge service started, listening on %s\n", port)

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

# HermesForge

HermesForge is a tiny AI Agent harness written from scratch in Go.

It provides:

- A minimal Agent Loop engine
- Unified LLM provider abstraction
- Built-in local tools such as bash and file operations
- Plugin adapters for Feishu, DingTalk, and WeChat bots
- A simple CLI entrypoint through `hforge`

## 核心设计哲学
- Harness over Framework: 真正的壁垒不在于调用大模型 API，而在于如何调度工具、管理上下文和安全拦截。
- 极简即是正义: 仅向大模型提供少量图灵完备原语。
- 状态外部化: 可将记忆与计划持久化到外部文档与文件系统。

## 当前基础架构
- `cmd/hf`: 进程入口，负责组装 provider、工具与主循环。
- `internal/engine`: Agent 主循环（最小 ReAct 流程）。
- `internal/provider`: 模型提供者抽象、provider registry 与通用模型配置项。
- `internal/tools`: 线程安全工具注册中心（注册、执行、枚举）。
- `internal/schema`: Provider / Engine 共享协议结构（消息与工具调用）。

## 快速开始
```bash
go run ./cmd/hforge
```

环境变量:
- `HF_PROVIDER`: provider 名称（`anthropic`/`openai`/`ollama`，默认 `anthropic`）
- `HF_MODEL`: 可选，覆盖默认模型
- `HF_API_KEY`: API key；优先级高于 provider 专用 key
- `HF_BASE_URL`: 可选，自定义兼容 API 地址
- `ANTHROPIC_API_KEY`: 使用 anthropic provider 时可作为 `HF_API_KEY` 的 fallback
- `OPENAI_API_KEY`: 使用 openai provider 时可作为 `HF_API_KEY` 的 fallback

MiniMax Anthropic-compatible API 示例:

```bash
# 飞书 webhook
export HF_PROVIDER=anthropic
export HF_BASE_URL=https://api.minimaxi.com/anthropic
export HF_MODEL=MiniMax-M2.7
export HF_API_KEY=''
export FEISHU_APP_ID=
export FEISHU_APP_SECRET=
export FEISHU_CONNECTION_MODE=websocket
go run ./cmd/hforge

# 钉钉 webhook
# endpoint: /webhook/dingtalk/event
export DINGTALK_CLIENT_SECRET=
go run ./cmd/hforge

# 微信 iLink long-poll
export WECHAT_BOT_TOKEN=
export WECHAT_ILINK_BOT_ID=
go run ./cmd/hforge

# 模型
export HF_PROVIDER=anthropic
export HF_BASE_URL=https://api.minimaxi.com/anthropic
export HF_MODEL=MiniMax-M2.7
export HF_API_KEY=''
go run ./cmd/smoke
```

Feishu uses WebSocket long connection by default, so it does not require a
public webhook URL. Set `FEISHU_CONNECTION_MODE=webhook` to use HTTP callbacks.

Webhook endpoints:

- Feishu: `/webhook/feishu/event`
- DingTalk: `/webhook/dingtalk/event`

The DingTalk webhook is enabled only when `DINGTALK_CLIENT_SECRET` is set.
`DINGTALK_APP_SECRET` is accepted as an alias.

WeChat uses iLink long-poll and starts only when `WECHAT_BOT_TOKEN` and
`WECHAT_ILINK_BOT_ID` are set.

## Provider 使用方式

### 通过 registry 使用

Provider 包通过 `init` 注册自己。入口程序只需要 blank import 对应 provider 包，然后用统一的 registry 创建实例:

```go
import (
	"github.com/teoclub/hermes-forge/internal/provider"

	_ "github.com/teoclub/hermes-forge/internal/provider/anthropic"
)

llmProvider, err := provider.New(
	"anthropic",
	provider.WithAPIKey(apiKey),
	provider.WithModel(modelName),
)
if err != nil {
	return err
}
```

`anthropic` provider 内部的注册代码:

```go
func init() {
	provider.MustRegisterProvider(providerName, New)
}

func New(opts ...provider.Option) (provider.LLMProvider, error) {
	return NewAnthropicProvider(opts...)
}
```

这种方式适合 `main.go` 这类需要按配置切换 provider 的入口。

### 直接使用具体 provider

如果代码已经确定只使用 Anthropic，也可以直接调用具体构造函数:

```go
import (
	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/provider/anthropic"
)

llmProvider, err := anthropic.NewAnthropicProvider(
	provider.WithAPIKey(apiKey),
	provider.WithModel(modelName),
)
if err != nil {
	return err
}
```

注意 `NewAnthropicProvider` 在 `anthropic` 包里，不是 `provider.NewAnthropicProvider`。

## 测试
```bash
go test ./...
```

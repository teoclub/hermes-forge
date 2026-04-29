# hermes-forge

a tiny AI Agent operating system built from scratch in Go.

这是一个基于“驾驭工程 (Harness Engineering)”理念，由 Go 语言从零实现的微型 AI Agent 操作系统。

## 核心设计哲学
- Harness over Framework: 真正的壁垒不在于调用大模型 API，而在于如何调度工具、管理上下文和安全拦截。
- 极简即是正义: 仅向大模型提供少量图灵完备原语。
- 状态外部化: 可将记忆与计划持久化到外部文档与文件系统。

## 当前基础架构
- `cmd/hf`: 进程入口，负责解析输入并启动应用。
- `internal/app`: 启动层（配置加载、依赖装配、内置工具注册）。
- `internal/engine`: Agent 主循环（最小 ReAct 流程）。
- `internal/provider`: 模型提供者抽象（当前内置 `MockProvider`）。
- `internal/tools`: 线程安全工具注册中心（注册、执行、枚举）。
- `internal/schema`: Provider / Engine 共享协议结构（消息与工具调用）。

## 快速开始
```bash
go run ./cmd/hf "hello"
go run ./cmd/hf "tool:upper hello world"
```

环境变量:
- `HF_MAX_STEPS`: Agent 循环最大步数（默认 `8`）

## 测试
```bash
go test ./...
```

## 下一步建议
- 接入真实 Provider（Claude/GLM/OpenAI）并在 `internal/provider` 下实现多后端。
- 在 `internal/tools` 增加超时、并发限流、审计日志。
- 引入 `plan memory` 与 `task memory` 的持久化模块。

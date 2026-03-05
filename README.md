# Aether: Enterprise-Grade Agentic OS 🚀

Aether 是一个基于 Go 语言构建的**企业级多智能体操作系统（MAS-OS）**。它采用 Clean Architecture 架构，旨在为企业提供可观测、可管控、自净化的 AI 智能体协作环境。

---

## ✨ 核心特性

- **🤖 多智能体协同 (MAS)**：内置 Supervisor, Planner, Coder, Reviewer 等角色，模拟真实组织架构执行复杂任务。
- **📊 工业级可观测性**：深度集成 OpenTelemetry (OTel) 与 Jaeger。支持全链路 TraceID 追踪，且日志与链路自动关联。
- **🛡️ 治理与安全 (Governance)**：基于数字宪法（Constitution）和风险护栏（Risk Guard）的执行策略，支持人机领航（Human-in-the-Loop）。
- **⚡ 实时流式反馈**：支持 Ollama/OpenAI 的实时流式输出，CLI 端具备打字机回显效果，告别“黑盒”等待。
- **🏠 本地优先 (Local-First)**：深度适配 Ollama (Qwen 3.5/2.5)，支持完全离线的隐私安全环境。
- **🛠️ 动态能力加载 (WASM)**：支持通过 WebAssembly 动态加载扩展 Skill，实现逻辑隔离与按需热更新。

---

## 🏗️ 系统架构

Aether 遵循 **Clean Architecture** 准则，分为以下层次：
- **Domain**: 核心业务逻辑、智能体状态机、治理规则。
- **Usecase**: DAG 执行引擎、学习引擎、反射闭环。
- **Infrastructure**: LLM 适配器 (Ollama/OpenAI)、NATS 总线、SQLite 存储。
- **Delivery**: 企业级 CLI (`aether task`)、REST API、Web UI。

---

## 🚀 快速开始

### 1. 准备环境
- 安装 [Go 1.22+](https://go.dev/)
- 启动本地 [Ollama](https://ollama.com/) 并拉取模型：`ollama pull qwen3.5:0.8b`
- （可选）启动 Jaeger 进行链路监控：
  ```bash
  docker run -d --name jaeger \
    -e COLLECTOR_OTLP_ENABLED=true \
    -p 16686:16686 -p 4317:4317 -p 4318:4318 \
    jaegertracing/all-in-one:latest
  ```

### 2. 编译并运行
```bash
# 编译二进制文件
go build -o aether cmd/aether/main.go

# 执行一个复杂的参数化任务
./aether task "Design a Pet Store API with Go and Fiber!"
```

### 3. 查看追踪
访问 **http://localhost:16686**，搜索 Service 为 `aether-core`。你将看到每一个 Agent 思考、执行、报错、自愈的全过程。

---

## 🛠️ 配置说明 (`configs/config.yaml`)

```yaml
app:
  mode: "single" # single/cluster
  role: "supervisor"

ollama:
  base_url: "http://localhost:11434"
  model: "qwen3.5:0.8b" # 推荐本地超轻量模型
  timeout: "300s"

otel:
  endpoint: "localhost:4317" # Jaeger OTLP 地址
```

---

## 📈 面试亮点：Agentic 自愈设计

在 Aether 中，报错不再是终点：
1. **Trace 快照**：系统自动将 LLM 的原始响应 (Raw Response) 捕获并存入 Span 属性。
2. **反射闭环**：当执行器捕获到 Panic 或 404 异常时，会自动触发 `Reflection` 智能体。
3. **数据驱动自愈**：Reflection 智能体通过 TraceID 提取故障快照，动态修正 Prompt 策略并触发重新执行。

---

## 📄 开源协议
本项目采用 [MIT License](LICENSE) 协议。

# Aether: Autonomous Agentic OS 🚀

Aether 是一个基于 Go 语言构建的**自主多智能体操作系统（MAS-OS）**。项目遵循 **Clean Architecture** 准则，旨在探索在高度动态环境下，AI 智能体集群的可观测性、自愈性与治理边界。

---

## 🎨 核心哲学

- **🧩 架构即艺术**：严格的领域驱动设计（DDD），确保系统在复杂任务流下依然保持逻辑的纯粹与解耦。
- **👁️ 全透明执行**：深度集成 OpenTelemetry (OTel)，将 Agent 的“思维链路”物化为可感知的追踪图谱。
- **🌀 自主进化循环**：通过执行-反射-学习的闭环设计，使系统具备处理不确定性异常的自愈能力。
- **🔌 动态能力边界**：利用 WebAssembly (WASM) 插件机制，实现 Agent 技能的零停机热扩展。

---

## ✨ 系统特性

- **🤖 多智能体协同**：内置 Supervisor, Planner, Coder, Reviewer 等角色，模拟组织化任务拆解。
- **📊 工业级可观测性**：链路追踪与结构化日志自动关联，支持 Jaeger 实时监控。
- **⚡ 实时流式反馈**：基于异步总线的 Token 广播，提供极致的 CLI 打字机交互体验。
- **🏠 本地化安全**：深度适配 Ollama 离线模型，保护数据主权与隐私。

---

## 🏗️ 快速开始

### 1. 环境准备
- **Go 1.22+**
- **Ollama**: `ollama pull qwen3.5:0.8b`
- **Jaeger** (可选): 用于可视化执行链路。

### 2. 编译并执行
```bash
# 编译
go build -o aether cmd/aether/main.go

# 下发自主任务
./aether task "Design a Pet Store API with Go and Fiber!"
```

---

## 🛠️ 技术内幕：自愈架构设计

在 Aether 的设计中，异常被视为系统进化的养料：
1. **执行快照**：每一个推理节点的 Input/Output 都会被实时序列化并作为 Span 属性持久化。
2. **故障自省**：当捕获到 Panic 或逻辑错误时，Reflection 引擎会自动回溯 Trace 属性中的故障现场。
3. **策略修正**：系统基于故障快照动态调整 Prompt 权重或重试策略，实现无人值守的闭环修复。

---

## 📄 协议
本项目采用 [MIT License](LICENSE) 协议。

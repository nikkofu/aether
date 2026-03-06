# Aether: The Autonomous Agentic OS 🌌

Aether 是一个基于 Go 语言构建的**艺术品级自主多智能体操作系统 (MAS-OS)**。它通过极致的 Clean Architecture 架构，将 AI 智能体的思维过程物化为可观测、可流转、可进化的全透明链路。

---

## 🎭 核心艺术美学

- **🌀 ReAct 推理原力**：每一个 Agent 都不再是简单的文本生成器，而是遵循 **Thought (思考) -> Action (行动) -> Observation (观察)** 的闭环生命体。
- **🌈 视觉思维链**：实时打字机特效结合 ANSI 颜色矩阵，将复杂的分布式 Agent 协作转化为一场视觉上的交响乐。
- **🛡️ 绝对鲁棒性**：工业级的故障自愈（Self-Healing）与 Panic 隔离机制，确保系统在处理不确定性任务时依然优雅。
- **👁️ 全息链路追踪**：深度集成 OpenTelemetry，每一丝思维闪念都在 Jaeger 图谱中清晰可见。

---

## ✨ 系统特性

- **🤖 多角色协同 (MAS)**：内置 Supervisor (指挥), Planner (谋略), Coder (构建), Reviewer (审计) 等核心角色。
- **⚡ 零延迟流式反馈**：基于异步消息总线的 Token 广播技术，提供极致流畅的 CLI 交互体验。
- **🏠 数据主权**：原生适配 Ollama (Qwen 3.5/2.5) 等本地模型，确保智慧在本地发生，安全在底层闭环。
- **🔌 插件化能力 (WASM)**：支持通过 WebAssembly 动态扩展 Agent 技能，逻辑隔离且热更新。

---

## 🏗️ 快速开始

### 1. 环境准备
- **Go 1.22+**
- **Ollama**: `ollama pull qwen3.5:0.8b`
- **Jaeger** (推荐): 用于可视化思维链路。

### 2. 编译并执行一键式任务
```bash
# 编译艺术品二进制文件
go build -o aether cmd/aether/main.go

# 开启一次自主架构设计任务
./aether task "Design a Pet Store Agentic Salesperson AI"
```

---

## 🎬 实战演示：ReAct 思维链

执行 `./aether task` 后，你将看到 Agent 如何像人类架构师一样思考：

> **[PLANNER]** 正在启动 ReAct 推理循环...  
> **Thought**: 深度分析任务目标。我们需要构建一个能够自主处理订单、生成推荐并优化销售策略的智能体。  
> **Action**: 拆解为以下模块：
> 1. 设计智能体核心决策框架。
> 2. 构建基于用户行为的推荐引擎。
> 3. 集成实时数据监控工具。  
> **Observation**: 定义交付标准：推荐准确率需 ≥ 90%，支持容器化部署。

---

## 🛠️ 技术深度：为什么它是艺术品？

在 Aether 中，代码不只是逻辑，更是可观测的艺术：
1. **统一路由模型**：所有 Agent 通过 NATS/MemoryBus 进行去中心化通信，Supervisor 进行旁路全局编排。
2. **状态机自净化**：执行失败会被自动转化为“经验实体”存入知识图谱，作为下次推理的 RAG 素材。
3. **优雅生命周期**：任务达成后，系统自动完成 Trace 刷新并优雅退出进程，不留一丝冗余。

---

## 📄 协议
本项目采用 [MIT License](LICENSE) 协议。

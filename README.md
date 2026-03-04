# Aether

> Agentic Engineering Orchestration System (AEOS)

Aether 是一个基于多智能体协作、具备高度可观测性和政策驱动能力的 AI 工程编排系统。它不仅仅是一个 LLM Wrapper，更是为了构建自动化的“AI 工程师团队”而设计的底层操作系统。

---

## 核心特性

- **多智能体编排 (Multi-Agent Orchestration)**: 内置 `Planner`, `Coder`, `Reviewer`, `Sentinel` 等专门角色的智能体，通过 `Manager` 和 `Supervisor` 进行协调。
- **DAG 任务执行与可视化**: 基于有向无环图的任务流转，支持复杂的任务依赖和并行处理，内置 Mermaid.js 格式图表导出 (`Pipeline.ToMermaid`) 用于透明化调试。
- **本地大模型支持 (Local LLM)**: 原生集成 Ollama 适配器，支持零成本、绝对隐私保护的本地断网环境运行，与 OpenAI/Gemini 并行作为底座。
- **可观测性 (Observability)**: 深度集成 OpenTelemetry，内置 Tracing 和 Metrics 系统，对每一个 Decision-to-Execution 链路进行全审计。
- **可扩展能力集 (Capabilities)**: 包含文件管理、日历、邮件、搜索、告警等多种原子能力，并支持通过 WASM 动态从远程扩展和加载 Skill。
- **治理与政策 (Governance & Policy)**: 具备基于投票的 DAO-like 治理委员会与自动执行的 `Policy Engine`，确保 AI 行为在安全边界内运行。
- **Web UI 管理界面**: 现代化的前端界面，用于任务监控、指标分析和代理配置。

- **事件驱动的自主守护进程 (Auto-Healing Daemon)**: 独立提供 `aetherd` 守护进程，可监听 GitHub 等平台的 Webhook 事件（如 Issue 创建），自动唤醒智能体集群进行代码分析与修复，实现“永不疲倦的 AI 队友”。

---

## 项目结构

```text
.
├── cmd/                # 应用程序入口
│   ├── aether/         # CLI 客户端
│   ├── aetherd/        # Webhook 事件驱动守护进程
│   └── observability_api/ # 可观测性服务

├── internal/           # 私有业务逻辑
│   ├── domain/         # 领域模型与接口 (Entities & Repository Interfaces)
│   ├── usecase/        # 业务逻辑编排 (Orchestration & Application Services)
│   ├── infrastructure/ # 外部基础设施实现 (LLM Clients, WASM Sandbox, DB)
│   ├── app/            # 应用程序引导 (Dependency Injection & Lifecycle)
│   └── delivery/       # 交付层入口 (CLI Commands, API Handlers)
├── pkg/                # 公共基础库 (可被外部项目引用的解耦组件)
│   ├── bus/            # 分布式事件总线
│   ├── logging/        # 结构化日志
│   └── observability/  # Tracing 与 Metrics 核心
├── deployments/        # 部署配置 (Docker, K8s)
├── web-ui/             # 基于 React 的管理前端
└── configs/            # 配置文件
```

---

## 快速开始

### 环境依赖

- Go 1.21+
- Node.js 18+
- SQLite (本地存储)
- NATS (可选，分布式部署需要)

### 编译运行

1. **编译后端**:
   ```bash
   go build -o aether ./cmd/aether
   ./aether --config configs/config.example.yaml
   ```

2. **启动 Web UI**:
   ```bash
   cd web-ui
   npm install
   npm run dev
   ```

---

## 路线图

- [x] 多智能体基础架构 (Agent Skeletion)
- [x] 可观测性系统 (Tracing & Metrics)
- [x] 基础能力集实现
- [x] 分布式调度系统 (NATS based)
- [x] WASM Skill 动态加载
- [x] 治理委员会投票系统 (DAO-like Governance)

---

## License

Apache-2.0

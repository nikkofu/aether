# Aether

> Agentic Engineering Orchestration System (AEOS)

Aether 是一个基于多智能体协作、具备高度可观测性和政策驱动能力的 AI 工程编排系统。它不仅仅是一个 LLM Wrapper，更是为了构建自动化的“AI 工程师团队”而设计的底层操作系统。

---

## 核心特性

- **多智能体编排 (Multi-Agent Orchestration)**: 内置 `Planner`, `Coder`, `Reviewer`, `Sentinel` 等专门角色的智能体，通过 `Manager` 和 `Supervisor` 进行协调。
- **DAG 任务执行**: 基于有向无环图的任务流转，支持复杂的任务依赖和并行处理。
- **可观测性 (Observability)**: 深度集成 OpenTelemetry，内置 Tracing 和 Metrics 系统，对每一个 Decision-to-Execution 链路进行全审计。
- **可扩展能力集 (Capabilities)**: 包含文件管理、日历、邮件、搜索、告警等多种原子能力，并支持通过 WASM 动态扩展 Skill。
- **治理与政策 (Governance & Policy)**: 具备 `Policy Engine` 和 `Evolution Guard`，确保 AI 行为在安全和业务政策边界内运行。
- **Web UI 管理界面**: 现代化的前端界面，用于任务监控、指标分析和代理配置。

---

## 项目结构

```text
.
├── cmd/                # 应用程序入口
│   ├── aether/         # 核心 CLI/运行时
│   ├── observability/  # 可观测性 API 服务
│   └── todo_api/       # 演示 API 示例
├── internal/           # 核心逻辑
│   ├── agent/          # 智能体逻辑与架构
│   ├── bus/            # 事件驱动总线 (Memory/NATS)
│   ├── capabilities/   # 原子能力集 (Search, File, etc.)
│   ├── observability/  # Tracing, Metrics 与监控
│   ├── org/            # 代理组织架构管理
│   └── skills/         # WASM 插件化技能系统
├── web-ui/             # 基于 React 的管理前端
└── configs/            # 示例配置文件
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
- [ ] WASM Skill 动态加载
- [ ] 治理委员会投票系统 (DAO-like Governance)

---

## License

Apache-2.0

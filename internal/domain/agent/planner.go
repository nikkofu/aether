package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/pkg/observability"
)

// PlannerAgent 负责任务拆解并注入执行策略。
type PlannerAgent struct {
	BaseAgent
	llm     capability.Capability
	tracer  observability.Tracer
	manager AgentManager
	graph   knowledge.Graph
}

func NewPlannerAgent(name string, llm capability.Capability, tracer observability.Tracer) *PlannerAgent {
	return &PlannerAgent{
		BaseAgent: *NewBaseAgent(name, "planner"),
		llm:       llm,
		tracer:    tracer,
	}
}

func (a *PlannerAgent) SetManager(m AgentManager) { a.manager = m }
func (a *PlannerAgent) SetGraph(g knowledge.Graph) { a.graph = g }

func (a *PlannerAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	// 1. 处理系统级消息
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	// 2. 核心逻辑包装在 ProtectedHandle 中以捕获 Panic
	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if msg.Type != "task_plan_request" {
			return nil, nil
		}

		// 检查核心依赖，防止 nil pointer
		if a.llm == nil {
			return nil, fmt.Errorf("planner agent error: LLM capability is not initialized")
		}

		description, _ := msg.Payload["description"].(string)
		if description == "" {
			return nil, fmt.Errorf("task description is empty")
		}

		fmt.Fprintf(os.Stderr, "\n🧠 [%s] 正在启动 ReAct 推理循环...\n", strings.ToUpper(a.name))

		// 3. 执行 LLM 推理 (ReAct 模式)
		input := map[string]any{
			"prompt": fmt.Sprintf(`你是一个资深的架构师智能体。请使用 ReAct 模式处理任务。
任务内容: '%s'

请严格按以下格式思考和输出：
Thought: 深度分析任务目标、技术选型难点及模块依赖。
Action: 将任务拆解为具体的执行指令（JSON 列表格式）。
Observation: 定义每个模块的成功交付标准。

开始推理：`, description),
			"agent_name": fmt.Sprintf("%s:reasoning", a.name),
			"stream":     true,
		}

		output, err := a.llm.Execute(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("LLM execution failed: %w", err)
		}

		// 4. 防御性提取结果
		if output == nil {
			return nil, fmt.Errorf("LLM returned nil output")
		}

		plan, ok := output["output"].(string)
		if !ok || plan == "" {
			return nil, fmt.Errorf("LLM did not return a valid plan string")
		}

		// 5. 生成后续指令（如果 Manager 已注入）
		// 注意：即便没有 Manager，我们也返回成功的消息列表，让总线继续流转
		resMsg := Message{
			From:      a.name,
			To:        "coder",
			Type:      "instruction",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"plan": plan,
				"task": description,
			},
		}

		// 如果 Manager 存在，则尝试 Spawn 一个 Coder
		if a.manager != nil {
			_ , _ = a.manager.Spawn(ctx, "coder", map[string]any{"task_id": msg.ID, "plan": plan})
		}

		return []Message{resMsg}, nil
	})
}

var _ Agent = (*PlannerAgent)(nil)

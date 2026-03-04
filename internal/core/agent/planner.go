package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/core/capability"
	"github.com/nikkofu/aether/internal/pkg/observability"
)

// PlannerAgent 负责任务拆解并注入执行策略。
type PlannerAgent struct {
	BaseAgent
	llm     capability.Capability
	tracer  observability.Tracer
	manager AgentManager
}

func NewPlannerAgent(name string, llm capability.Capability, tracer observability.Tracer) *PlannerAgent {
	return &PlannerAgent{
		BaseAgent: *NewBaseAgent(name, "planner"),
		llm:       llm,
		tracer:    tracer,
	}
}

func (a *PlannerAgent) SetManager(m AgentManager) { a.manager = m }

func (a *PlannerAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if a.tracer != nil {
			var span observability.Span
			ctx, span = a.tracer.StartSpan(ctx, "Planner.Handle", map[string]any{"type": msg.Type})
			defer span.End()
		}

		if msg.Type != "task_plan_request" { return nil, nil }

		description, _ := msg.Payload["description"].(string)

		// 注入 agent_name 以便 Skill 获取策略
		input := map[string]any{
			"prompt":     fmt.Sprintf("拆解任务：'%s'。按模块分工。", description),
			"agent_name": a.name,
		}
		
		output, err := a.llm.Execute(ctx, input)
		if err != nil { return nil, err }
		
		plan, _ := output["output"].(string)

		if a.manager != nil {
			_, _ = a.manager.Spawn(ctx, "coder", map[string]any{"task_id": msg.ID})
		}

		return []Message{{
			From:      a.name,
			To:        "coder",
			Type:      "instruction",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"plan": plan,
				"task": description,
			},
		}}, nil
	})
}

var _ Agent = (*PlannerAgent)(nil)

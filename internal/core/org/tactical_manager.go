package org

import (
	"context"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/core/agent"
	"github.com/nikkofu/aether/internal/core/capability"
	"github.com/nikkofu/aether/internal/pkg/observability"
)

// TacticalManagerAgent 负责战术执行，将里程碑拆解为任务并指派工人。
type TacticalManagerAgent struct {
	agent.BaseAgent
	supervisor   string
	subs         []string
	agentManager agent.AgentManager
	llm          capability.Capability
	tracer       observability.Tracer
}

func NewTacticalManagerAgent(id string, supervisor string, am agent.AgentManager, llm capability.Capability, t observability.Tracer) *TacticalManagerAgent {
	return &TacticalManagerAgent{
		BaseAgent:    *agent.NewBaseAgent(id, "tactical-manager"),
		supervisor:   supervisor,
		agentManager: am,
		llm:          llm,
		tracer:       t,
	}
}

func (a *TacticalManagerAgent) ID() string           { return a.Name() }
func (a *TacticalManagerAgent) Level() OrgLevel      { return LevelTactical }
func (a *TacticalManagerAgent) Supervisor() string    { return a.supervisor }
func (a *TacticalManagerAgent) Subordinates() []string { return a.subs }

func (a *TacticalManagerAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		if msg.Type == "milestone.assigned" {
			milestone, _ := msg.Payload["milestone"].(map[string]any)
			goalID, _ := msg.Payload["goal_id"].(string)

			// 1. 拆解里程碑为任务
			input := map[string]any{
				"prompt": fmt.Sprintf("将里程碑 '%s' 拆解为 2-3 个具体的 Go 开发任务。", milestone["title"]),
			}
			output, err := a.llm.Execute(ctx, input)
			if err != nil { return nil, err }
			
			tasks := output["output"].(string)

			// 2. 动态 Spawn 执行工人
			workerName, err := a.agentManager.Spawn(ctx, "operational", map[string]any{"task_id": msg.ID})
			if err != nil { return nil, err }

			// 3. 下发任务给工人
			return []agent.Message{{
				From: a.Name(),
				To:   workerName.Name(),
				Type: "task.assigned",
				Timestamp: time.Now(),
				Payload: map[string]any{
					"tasks":   tasks,
					"goal_id": goalID,
					"ms_id":   milestone["id"],
				},
			}}, nil
		}

		if msg.Type == "task.completed" {
			// 4. 聚合结果并汇报战略主管
			return []agent.Message{{
				From: a.Name(),
				To:   a.supervisor,
				Type: "milestone.feedback",
				Timestamp: time.Now(),
				Payload: map[string]any{
					"success": true,
					"goal_id": msg.Payload["goal_id"],
					"ms_id":   msg.Payload["ms_id"],
				},
			}}, nil
		}

		return nil, nil
	})
}

var _ OrgAgent = (*TacticalManagerAgent)(nil)

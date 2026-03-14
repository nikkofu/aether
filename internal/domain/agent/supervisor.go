package agent

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/pkg/observability"
)

// SupervisorAgent 负责编排和自进化决策。
type SupervisorAgent struct {
	BaseAgent
	tracer observability.Tracer
	logger logging.Logger
	graph  knowledge.Graph
}

func NewSupervisorAgent(name string, t observability.Tracer, l logging.Logger) *SupervisorAgent {
	return &SupervisorAgent{
		BaseAgent: *NewBaseAgent(name, "supervisor"),
		tracer:    t,
		logger:    l,
	}
}

func (a *SupervisorAgent) SetGraph(g knowledge.Graph) { a.graph = g }

func (a *SupervisorAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	// 优先处理系统级消息
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if a.tracer != nil {
			var span observability.Span
			ctx, span = a.tracer.StartSpan(ctx, "Supervisor.Handle", map[string]any{"type": msg.Type})
			defer span.End()
		}

		switch msg.Type {
		case "final_report":
			if a.logger != nil {
				a.logger.Info(ctx, "收到工作流最终交付",
					logging.String("task_id", stringValue(msg.Payload["task_id"])),
					logging.String("from", msg.From),
				)
			}
			return nil, nil

		case "system.alert":
			return a.handleAlert(ctx, msg)

		case "agent.reflection":
			return a.handleReflection(ctx, msg)

		case "work_progress":
			// 汇总进度
			status, _ := msg.Payload["status"].(string)
			if a.logger != nil {
				a.logger.Info(ctx, "收到工作进度上报", logging.String("agent", msg.From), logging.String("progress", status))
			}
			return nil, nil
		}

		return nil, nil
	})
}

func (a *SupervisorAgent) handleReflection(ctx context.Context, msg Message) ([]Message, error) {
	if a.graph == nil {
		return nil, nil
	}

	orgID, _ := msg.Payload["org_id"].(string)
	if orgID == "" {
		orgID = "default"
	}

	analysis, _ := msg.Payload["analysis"].(string)
	agentName, _ := msg.Payload["agent_name"].(string)

	// 将反思作为经验存入知识图谱
	entity := knowledge.Entity{
		ID:   "refl-" + msg.ID,
		Type: "reflection",
		Name: "Reflection from " + agentName,
		Metadata: map[string]any{
			"analysis":   analysis,
			"agent_name": agentName,
			"task_id":    msg.Payload["task_id"],
			"success":    msg.Payload["success"],
		},
		CreatedAt: time.Now(),
	}

	_ = a.graph.AddEntity(ctx, entity, orgID)

	if a.logger != nil {
		a.logger.Info(ctx, "已记录 Agent 历史工程经验", logging.String("agent", agentName))
	}

	return nil, nil
}

func (a *SupervisorAgent) handleAlert(ctx context.Context, msg Message) ([]Message, error) {
	severity, _ := msg.Payload["severity"].(string)
	message, _ := msg.Payload["message"].(string)
	if a.logger != nil {
		if severity == "CRITICAL" || severity == "HIGH" {
			a.logger.Error(ctx, "收到工作流告警",
				logging.String("severity", severity),
				logging.String("task_id", stringValue(msg.Payload["task_id"])),
				logging.String("origin", msg.From),
				logging.String("message", message),
			)
		} else {
			a.logger.Warn(ctx, "收到工作流告警",
				logging.String("severity", severity),
				logging.String("task_id", stringValue(msg.Payload["task_id"])),
				logging.String("origin", msg.From),
				logging.String("message", message),
			)
		}
	}

	return nil, nil
}

var _ Agent = (*SupervisorAgent)(nil)

package agent

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/pkg/observability"
)

// SupervisorAgent 负责编排和自进化决策。
type SupervisorAgent struct {
	BaseAgent
	mu      sync.Mutex
	tracer  observability.Tracer
	logger  logging.Logger
	retries map[string]int
	graph   knowledge.Graph
}

const MaxRetryLimit = 3

func NewSupervisorAgent(name string, t observability.Tracer, l logging.Logger) *SupervisorAgent {
	return &SupervisorAgent{
		BaseAgent: *NewBaseAgent(name, "supervisor"),
		tracer:    t,
		logger:    l,
		retries:   make(map[string]int),
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
		case "task":
			return []Message{{
				From:      a.name,
				To:        "planner",
				Type:      "task_plan_request",
				Timestamp: time.Now(),
				Payload:   msg.Payload,
			}}, nil

		case "instruction":
			// 当 Planner 完成计划后，Supervisor 负责调度 Coder 开始执行
			fmt.Fprintf(os.Stderr, "\n\n\033[1;35m📡 [SUPERVISOR]\033[0m 任务计划已就绪，正在指派 \033[1;34m[CODER]\033[0m 执行开发逻辑...\n")
			fmt.Fprintf(os.Stderr, "\033[1;34m[CODER]\033[0m 正在根据计划编写代码并自测中...\n")
			return nil, nil 

		case "review_result":
			approved, _ := msg.Payload["approved"].(bool)
			if approved {
				fmt.Fprintf(os.Stderr, "\n\n\033[1;32m🏁 [SUPERVISOR] 核心链路通过评审！生成最终交付报告...\033[0m\n")
				return []Message{{
					From:      a.name,
					To:        "cli-feedback",
					Type:      "final_report",
					Timestamp: time.Now(),
					Payload:   map[string]any{"result": "任务已圆满完成。"},
				}}, nil
			}
			fmt.Fprintf(os.Stderr, "\n\033[1;33m⚠️ [SUPERVISOR] 评审未通过，正在调度 [CODER] 进行迭代修复...\033[0m\n")
			return nil, nil

		case "final_report":
			a.SetStatus(StatusCompleted)
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
	if a.graph == nil { return nil, nil }

	orgID, _ := msg.Payload["org_id"].(string)
	if orgID == "" { orgID = "default" }

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
	a.mu.Lock()
	defer a.mu.Unlock()

	severity, _ := msg.Payload["severity"].(string)
	originMsgID, _ := msg.Payload["origin_id"].(string)

	if severity == "CRITICAL" {
		count := a.retries[originMsgID]
		if count < MaxRetryLimit {
			a.retries[originMsgID] = count + 1
			a.logger.Warn(ctx, "触发自愈重试", logging.Int("retry_count", count+1))
			return []Message{{
				From:      a.name,
				To:        "planner",
				Type:      "task_plan_request",
				Timestamp: time.Now(),
				Payload:   map[string]any{"description": "自愈重试任务", "agent_name": a.name},
			}}, nil
		}
		a.SetStatus(StatusFailed)
	}

	return nil, nil
}

var _ Agent = (*SupervisorAgent)(nil)

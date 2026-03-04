package agent

import (
	"context"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/logging"
	"github.com/nikkofu/aether/internal/observability"
)

// SupervisorAgent 负责编排和自进化决策。
type SupervisorAgent struct {
	BaseAgent
	mu      sync.Mutex
	tracer  observability.Tracer
	logger  logging.Logger
	retries map[string]int
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

func (a *SupervisorAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
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

		case "final_report":
			a.SetStatus(StatusCompleted)
			return nil, nil

		case "system.alert":
			return a.handleAlert(ctx, msg)
		}

		return nil, nil
	})
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

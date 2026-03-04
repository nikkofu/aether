package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/capability"
	"github.com/nikkofu/aether/internal/observability"
)

// ReviewerAgent 负责代码评审。
type ReviewerAgent struct {
	BaseAgent
	llmSkill capability.Capability
	tracer   observability.Tracer
}

func NewReviewerAgent(name string, llm capability.Capability, tracer observability.Tracer) *ReviewerAgent {
	return &ReviewerAgent{
		BaseAgent: *NewBaseAgent(name, "reviewer"),
		llmSkill:  llm,
		tracer:    tracer,
	}
}

func (a *ReviewerAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if a.tracer != nil {
			var span observability.Span
			ctx, span = a.tracer.StartSpan(ctx, "Reviewer.Handle", map[string]any{"type": msg.Type})
			defer span.End()
		}

		if msg.Type != "review_request" { return nil, nil }

		code := msg.Payload["code"].(string)
		task := msg.Payload["task"].(string)

		input := map[string]any{
			"prompt":     fmt.Sprintf("评审针对任务 '%s' 的代码：\n%s", task, code),
			"agent_name": a.name,
		}

		output, err := a.llmSkill.Execute(ctx, input)
		if err != nil { return nil, err }

		review, _ := output["output"].(string)

		return []Message{{
			From:      a.name,
			Type:      "final_report",
			Timestamp: time.Now(),
			Payload:   map[string]any{"review": review, "code": code},
		}}, nil
	})
}

var _ Agent = (*ReviewerAgent)(nil)

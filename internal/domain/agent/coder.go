package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/pkg/observability"
)

// CoderAgent 负责编写代码并支持策略优化。
type CoderAgent struct {
	BaseAgent
	llmSkill capability.Capability
	tracer   observability.Tracer
}

func NewCoderAgent(name string, llm capability.Capability, tracer observability.Tracer) *CoderAgent {
	return &CoderAgent{
		BaseAgent: *NewBaseAgent(name, "coder"),
		llmSkill:  llm,
		tracer:    tracer,
	}
}

func (a *CoderAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	// 优先处理系统级消息
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if a.tracer != nil {
			var span observability.Span
			ctx, span = a.tracer.StartSpan(ctx, "Coder.Handle", map[string]any{"type": msg.Type})
			defer span.End()
		}

		if msg.Type != "instruction" { return nil, nil }

		plan := msg.Payload["plan"].(string)
		task := msg.Payload["task"].(string)

		input := map[string]any{
			"prompt":     fmt.Sprintf("基于规划实现代码：\n任务：%s\n规划：%s", task, plan),
			"agent_name": a.name,
		}

		output, err := a.llmSkill.Execute(ctx, input)
		if err != nil { return nil, err }

		code, _ := output["output"].(string)

		return []Message{{
			From:      a.name,
			To:        "reviewer",
			Type:      "review_request",
			Timestamp: time.Now(),
			Payload:   map[string]any{"code": code, "task": task},
		}}, nil
	})
}

var _ Agent = (*CoderAgent)(nil)

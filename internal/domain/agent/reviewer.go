package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/pkg/observability"
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
	// 优先处理系统级消息
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if a.tracer != nil {
			var span observability.Span
			ctx, span = a.tracer.StartSpan(ctx, "Reviewer.Handle", map[string]any{
				"task": msg.Payload["task"],
			})
			defer span.End()

			if msg.Type != "review_request" { return nil, nil }

			code := msg.Payload["code"].(string)
			task := msg.Payload["task"].(string)

			prompt := fmt.Sprintf("作为高级架构师，请评审针对任务 '%s' 的代码，给出详细意见并判定是否通过：\n%s", task, code)
			
			// 记录 LLM 调用
			llmCtx, llmSpan := a.tracer.StartSpan(ctx, "Reviewer.LLM_Inference", map[string]any{
				"code_to_review": code,
			})
			output, err := a.llmSkill.Execute(llmCtx, map[string]any{"prompt": prompt, "agent_name": a.name})
			if err != nil {
				llmSpan.End()
				return nil, err
			}
			review, _ := output["output"].(string)
			llmSpan.End()

			// 记录评审结果
			_, resSpan := a.tracer.StartSpan(ctx, "Reviewer.Result", map[string]any{
				"review_feedback": review,
			})
			resSpan.End()

			// 逻辑：向 Coder 反馈评审结果
			return []Message{{
				From:      a.name,
				To:        msg.From, // 回复给 Coder
				Type:      "review_result",
				Timestamp: time.Now(),
				Payload: map[string]any{
					"approved": true, // 简化逻辑，暂定为 true
					"feedback": review,
					"code":     code,
				},
			}}, nil
		}
		return nil, nil
	})
}

var _ Agent = (*ReviewerAgent)(nil)

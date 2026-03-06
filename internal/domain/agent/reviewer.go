package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
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
		if msg.Type != "review_request" { return nil, nil }

		code, _ := msg.Payload["code"].(string)
		task, _ := msg.Payload["task"].(string)

		fmt.Fprintf(os.Stderr, "\n🧐 [%s] 正在进行深度质量评审...\n", strings.ToUpper(a.name))

		prompt := fmt.Sprintf(`作为资深架构师，请对以下代码进行 ReAct 模式评审：
任务背景: %s
待审代码:
%s

请按以下格式输出：
Thought: 分析代码的逻辑正确性、安全性和性能。
Decision: [PASS] 或 [FAIL]
Feedback: 详细的改进意见。

开始评审：`, task, code)
		
		output, err := a.llmSkill.Execute(ctx, map[string]any{
			"prompt":     prompt, 
			"agent_name": fmt.Sprintf("%s:reviewing", a.name),
			"stream":     true,
		})
		if err != nil { return nil, err }

		review, _ := output["output"].(string)
		approved := !strings.Contains(strings.ToUpper(review), "DECISION: [FAIL]")

		// 如果未通过，在终端给予醒目提示
		if !approved {
			fmt.Fprintf(os.Stderr, "\n\033[1;31m❌ 评审未通过，已打回重做！\033[0m\n")
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[1;32m✅ 评审通过！\033[0m\n")
		}

		return []Message{{
			From:      a.name,
			To:        msg.From, // 回复给发送方
			Type:      "review_result",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"approved": approved,
				"feedback": review,
				"code":     code,
			},
		}}, nil
	})
}

var _ Agent = (*ReviewerAgent)(nil)

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
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
			ctx, span = a.tracer.StartSpan(ctx, "Coder.Handle", map[string]any{
				"task_desc": msg.Payload["task"],
				"plan":      msg.Payload["plan"],
			})
			defer span.End()
		}

		if msg.Type == "instruction" {
			plan := msg.Payload["plan"].(string)
			task := msg.Payload["task"].(string)
			taskID, _ := msg.Payload["task_id"].(string)
			deliveryTarget := "reviewer"
			if target, ok := msg.Payload["delivery_target"].(string); ok && target != "" {
				deliveryTarget = target
			}
			deliveryType := "review_request"
			if typed, ok := msg.Payload["delivery_type"].(string); ok && typed != "" {
				deliveryType = typed
			}
			progressTarget := "supervisor"
			if target, ok := msg.Payload["progress_target"].(string); ok && target != "" {
				progressTarget = target
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			contract := taskdomain.ExtractOutputContract(task)
			prompt := buildCoderPrompt(task, plan, contract)

			// 记录 LLM 动作
			llmCtx := ctx
			var llmSpan observability.Span
			if a.tracer != nil {
				llmCtx, llmSpan = a.tracer.StartSpan(ctx, "Coder.LLM_Inference", map[string]any{
					"prompt": prompt,
				})
			}
			output, err := a.llmSkill.Execute(llmCtx, map[string]any{"prompt": prompt, "agent_name": a.name, "task_id": taskID})
			if err != nil {
				if llmSpan != nil {
					llmSpan.End()
				}
				return nil, err
			}
			code, _ := output["output"].(string)
			if llmSpan != nil {
				llmSpan.End()
			}

			// 记录生成结果
			if a.tracer != nil {
				_, resSpan := a.tracer.StartSpan(ctx, "Coder.Output", map[string]any{
					"generated_content": code,
				})
				if resSpan != nil {
					resSpan.End()
				}
			}

			if err := ctx.Err(); err != nil {
				return nil, err
			}

			return []Message{
				{
					ID:        msg.ID,
					From:      a.name,
					To:        deliveryTarget,
					Type:      deliveryType,
					Timestamp: time.Now(),
					Payload: map[string]any{
						"code":      code,
						"task":      task,
						"task_id":   taskID,
						"trace_id":  msg.Payload["trace_id"],
						"iteration": msg.Payload["iteration"],
					},
				},
				{
					ID:        msg.ID,
					From:      a.name,
					To:        progressTarget,
					Type:      "work_progress",
					Timestamp: time.Now(),
					Payload: map[string]any{
						"status":  "completed_coding",
						"code":    code,
						"task_id": taskID,
					},
				},
			}, nil
		}

		// 处理来自 Reviewer 的反馈
		if msg.Type == "review_result" {
			approved, _ := msg.Payload["approved"].(bool)
			if approved {
				// 如果审核通过，Coder 负责发布最终报告给 CLI
				return []Message{{
					ID:        msg.ID,
					From:      a.name,
					To:        "cli",
					Type:      "final_report",
					Timestamp: time.Now(),
					Payload: map[string]any{
						"result":  msg.Payload["code"],
						"task_id": msg.Payload["task_id"],
					},
				}}, nil
			}
		}

		return nil, nil
	})
}

func buildCoderPrompt(task, plan string, contract taskdomain.OutputContract) string {
	if useChinesePrompt(task, plan) {
		return fmt.Sprintf(`你是 Aether 的交付执行代理。请基于任务与规划直接生成最终交付物。
任务：%s
规划或修订指令：%s
显式交付约束：%s

严格规则：
1. 只输出最终交付物，不要解释你将做什么，不要复述规划，不要添加前言、标题、总结或自我说明。
2. 如果任务要求精确的项目符号数量、清单数量、精确前缀或词数上限，必须严格满足。
3. 如果规划里包含 review feedback 或 deterministic validation，必须逐条修正。
4. 除非任务明确要求，否则不要输出 Markdown 代码块。
5. 不要输出“下面是最终结果”“我将生成交付物”等元叙述。%s

现在直接给出最终交付物：`, task, plan, taskdomain.DescribeOutputContract(contract), buildCoderContractExecutionGuidance(contract, true))
	}

	return fmt.Sprintf(`You are Aether's delivery agent. Generate the final deliverable directly from the task and revision instructions.
Task: %s
Plan or revision instructions: %s
Explicit output contract: %s

Strict rules:
1. Output only the final deliverable. Do not explain what you will do. Do not restate the plan. Do not add a preface, heading, conclusion, or self-reference unless the task explicitly asks for it.
2. If the task requires an exact bullet count, checklist count, exact prefixes, or a word limit, satisfy those constraints exactly.
3. If the revision instructions contain review feedback or deterministic validation failures, fix every listed issue.
4. Do not use Markdown code fences unless the task explicitly requires them.
5. Never output meta text such as "Here is the final deliverable" or "I will generate the final deliverable."%s

Return only the final deliverable:`, task, plan, taskdomain.DescribeOutputContractEnglish(contract), buildCoderContractExecutionGuidance(contract, false))
}

func buildCoderContractExecutionGuidance(contract taskdomain.OutputContract, chinese bool) string {
	if len(contract.ExactBulletPrefixes) == 0 {
		return ""
	}

	var builder strings.Builder
	if chinese {
		builder.WriteString("\n6. 如果存在精确项目符号前缀要求，必须按给定顺序逐行输出，并且每个前缀只占一行。")
		builder.WriteString("\n7. 绝不能把多个必需前缀合并到同一行。")
		if len(contract.BulletPhraseRequirements) > 0 {
			builder.WriteString("\n8. 如果某一行要求必须提到指定短语，必须原样保留该短语。")
		}
	} else {
		builder.WriteString("\n6. When exact bullet prefixes are required, output one bullet line per required prefix in the specified order.")
		builder.WriteString("\n7. Never place multiple required prefixes on the same line.")
		if len(contract.BulletPhraseRequirements) > 0 {
			builder.WriteString("\n8. Preserve each required phrase verbatim inside its matching bullet.")
		}
	}

	skeleton := buildCoderBulletSkeleton(contract, chinese)
	if skeleton == "" {
		return builder.String()
	}

	if chinese {
		builder.WriteString("\n\n逐行输出骨架：\n")
		builder.WriteString(skeleton)
		builder.WriteString("\n请沿用上述逐行结构；如果某行需要补充内容，只能在对应前缀后追加与任务事实一致的简洁内容。")
		return builder.String()
	}

	builder.WriteString("\n\nLine-by-line output skeleton:\n")
	builder.WriteString(skeleton)
	builder.WriteString("\nUse the same line structure above. If a line needs extra detail, add only concise task facts after the matching prefix.")
	return builder.String()
}

func buildCoderBulletSkeleton(contract taskdomain.OutputContract, chinese bool) string {
	if len(contract.ExactBulletPrefixes) == 0 {
		return ""
	}

	requiredPhraseByPrefix := make(map[string]string, len(contract.BulletPhraseRequirements))
	for _, requirement := range contract.BulletPhraseRequirements {
		if _, exists := requiredPhraseByPrefix[requirement.Prefix]; exists {
			continue
		}
		requiredPhraseByPrefix[requirement.Prefix] = strings.TrimSpace(requirement.RequiredPhrase)
	}

	lines := make([]string, 0, len(contract.ExactBulletPrefixes))
	for _, prefix := range contract.ExactBulletPrefixes {
		line := strings.TrimSpace(prefix)
		if line == "" {
			continue
		}
		content := requiredPhraseByPrefix[prefix]
		if content == "" {
			if chinese {
				content = "在此填写符合任务事实的简洁内容"
			} else {
				content = "replace with concise task facts"
			}
		}
		if !strings.HasSuffix(line, " ") {
			line += " "
		}
		lines = append(lines, line+content)
	}

	return strings.Join(lines, "\n")
}

var _ Agent = (*CoderAgent)(nil)

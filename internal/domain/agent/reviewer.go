package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
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
		if msg.Type != "review_request" {
			return nil, nil
		}

		deliverable, _ := msg.Payload["code"].(string)
		task, _ := msg.Payload["task"].(string)
		taskID, _ := msg.Payload["task_id"].(string)
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		fmt.Fprintf(os.Stderr, "\n🧐 [%s] 正在进行深度质量评审...\n", strings.ToUpper(a.name))

		contract := taskdomain.ExtractOutputContract(task)
		chinesePrompt := useChinesePrompt(task, deliverable)
		deliverableViolations := validateDeliverableWithLocale(contract, deliverable, chinesePrompt)
		if !contract.IsZero() {
			return []Message{a.buildDeterministicContractReviewResult(msg, task, taskID, deliverable, contract, deliverableViolations, chinesePrompt)}, nil
		}

		prompt := buildReviewerPrompt(task, contract, deliverable, chinesePrompt)

		output, err := a.llmSkill.Execute(ctx, map[string]any{
			"prompt":     prompt,
			"agent_name": fmt.Sprintf("%s:reviewing", a.name),
			"task_id":    taskID,
			"iteration":  msg.Payload["iteration"],
			"stream":     true,
		})
		if err != nil {
			return nil, err
		}

		review, _ := output["output"].(string)
		approved, explicitDecision := parseReviewDecision(review)
		decisionSource := "llm"
		reviewerProtocolViolations := []string(nil)
		if !explicitDecision {
			if chinesePrompt {
				reviewerProtocolViolations = append(reviewerProtocolViolations, "评审结果缺少明确的 `Decision: [PASS]` 或 `Decision: [FAIL]` 行。")
			} else {
				reviewerProtocolViolations = append(reviewerProtocolViolations, "The review result is missing an explicit `Decision: [PASS]` or `Decision: [FAIL]` line.")
			}

			repairedApproved, repairedExplicit := a.repairReviewDecision(ctx, task, taskID, contract, deliverable, strings.TrimSpace(review), msg.Payload["iteration"], chinesePrompt)
			switch {
			case repairedExplicit:
				approved = repairedApproved
				explicitDecision = true
				decisionSource = "repair"
			case len(deliverableViolations) == 0 && !contract.IsZero():
				approved = true
				explicitDecision = true
				decisionSource = "contract_fallback"
			default:
				if inferredApproved, inferred := inferReviewDecision(review); inferred {
					approved = inferredApproved
					explicitDecision = true
					decisionSource = "heuristic"
				} else {
					approved = false
					decisionSource = "missing"
				}
			}
		}

		feedback := strings.TrimSpace(review)
		revisionFeedback := feedback
		if len(deliverableViolations) > 0 {
			approved = false
			feedback = mergeReviewFeedback(review, deliverableViolations)
			revisionFeedback = buildRevisionFeedback(task, contract, deliverableViolations, chinesePrompt)
		}

		// 如果未通过，在终端给予醒目提示
		if !approved {
			fmt.Fprintf(os.Stderr, "\n\033[1;31m❌ 评审未通过，已打回重做！\033[0m\n")
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[1;32m✅ 评审通过！\033[0m\n")
		}

		return []Message{{
			ID:        msg.ID,
			From:      a.name,
			To:        msg.From, // 回复给发送方
			Type:      "review_result",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"approved":                     approved,
				"feedback":                     feedback,
				"revision_feedback":            revisionFeedback,
				"code":                         deliverable,
				"task_id":                      taskID,
				"quality_gate_violations":      deliverableViolations,
				"reviewer_protocol_violations": reviewerProtocolViolations,
				"review_decision_source":       decisionSource,
			},
		}}, nil
	})
}

func (a *ReviewerAgent) buildDeterministicContractReviewResult(msg Message, task, taskID, deliverable string, contract taskdomain.OutputContract, deliverableViolations []string, chinese bool) Message {
	approved := len(deliverableViolations) == 0
	decisionSource := "deterministic_contract_pass"
	feedback := deterministicContractFeedback(contract, approved, chinese)
	revisionFeedback := feedback
	if !approved {
		decisionSource = "deterministic_contract_fail"
		feedback = mergeReviewFeedback("", deliverableViolations)
		revisionFeedback = buildRevisionFeedback(task, contract, deliverableViolations, chinese)
		fmt.Fprintf(os.Stderr, "\n\033[1;31m❌ 评审未通过，已打回重做！\033[0m\n")
	} else {
		fmt.Fprintf(os.Stderr, "\n\033[1;32m✅ 评审通过！\033[0m\n")
	}

	return Message{
		ID:        msg.ID,
		From:      a.name,
		To:        msg.From,
		Type:      "review_result",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"approved":                     approved,
			"feedback":                     feedback,
			"revision_feedback":            revisionFeedback,
			"code":                         deliverable,
			"task_id":                      taskID,
			"quality_gate_violations":      deliverableViolations,
			"reviewer_protocol_violations": []string{},
			"review_decision_source":       decisionSource,
		},
	}
}

func buildReviewerPrompt(task string, contract taskdomain.OutputContract, deliverable string, chinese bool) string {
	if chinese {
		return fmt.Sprintf(`作为资深交付审查员，请对以下交付物进行严格评审：
任务背景: %s
显式交付约束: %s
待审交付物:
%s

请重点检查：
1. 交付物是否真正回答了任务，而不是复述计划、思考过程或元说明。
2. 交付物是否满足显式格式、长度、精确前缀和清单数量约束。
3. 交付物是否存在明显跑题、空洞套话或“我将要做什么”式表述。

请严格按以下格式输出，不要省略 Decision 行：
Thought: 分析交付物的任务完成度、约束符合度和质量风险。
Decision: [PASS] 或 [FAIL]
Feedback: 详细的改进意见。

开始评审：`, task, taskdomain.DescribeOutputContract(contract), deliverable)
	}

	return fmt.Sprintf(`You are a strict deliverable reviewer. Review the deliverable against the task.
Task: %s
Explicit output contract: %s
Deliverable:
%s

Focus on:
1. Whether the deliverable actually answers the task instead of restating a plan, chain-of-thought, or meta commentary.
2. Whether the deliverable satisfies the explicit format, length, exact-prefix, and checklist constraints.
3. Whether the deliverable is off-topic, vague, empty, or written as "I will do ..." instead of the final result.

You must output exactly this structure and you must include the Decision line:
Thought: Analyze task completion, contract compliance, and quality risks.
Decision: [PASS] or [FAIL]
Feedback: Specific revision guidance.

Start the review:`, task, taskdomain.DescribeOutputContractEnglish(contract), deliverable)
}

func parseReviewDecision(review string) (approved bool, explicit bool) {
	normalized := strings.ToUpper(review)

	switch {
	case strings.Contains(normalized, "DECISION: [FAIL]"), strings.Contains(normalized, "DECISION: FAIL"):
		return false, true
	case strings.Contains(normalized, "DECISION: [PASS]"), strings.Contains(normalized, "DECISION: PASS"):
		return true, true
	default:
		return false, false
	}
}

func validateDeliverableWithLocale(contract taskdomain.OutputContract, deliverable string, chinese bool) []string {
	if chinese {
		return taskdomain.ValidateOutputAgainstContract(contract, deliverable)
	}
	return taskdomain.ValidateOutputAgainstContractEnglish(contract, deliverable)
}

func mergeReviewFeedback(review string, violations []string) string {
	base := strings.TrimSpace(review)
	if len(violations) == 0 {
		return base
	}

	var builder strings.Builder
	if base != "" {
		builder.WriteString(base)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Deterministic Validation:\n")
	for _, violation := range violations {
		trimmed := strings.TrimSpace(violation)
		if trimmed == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(trimmed)
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func deterministicContractFeedback(contract taskdomain.OutputContract, approved bool, chinese bool) string {
	if chinese {
		if approved {
			return fmt.Sprintf("Deterministic contract review passed. 显式交付约束已满足：%s", taskdomain.DescribeOutputContract(contract))
		}
		return fmt.Sprintf("Deterministic contract review failed. 显式交付约束未满足：%s", taskdomain.DescribeOutputContract(contract))
	}
	if approved {
		return fmt.Sprintf("Deterministic contract review passed. Explicit output contract satisfied: %s", taskdomain.DescribeOutputContractEnglish(contract))
	}
	return fmt.Sprintf("Deterministic contract review failed. Explicit output contract was not satisfied: %s", taskdomain.DescribeOutputContractEnglish(contract))
}

func buildRevisionFeedback(task string, contract taskdomain.OutputContract, violations []string, chinese bool) string {
	var builder strings.Builder
	if chinese {
		builder.WriteString("请基于以下要求重新生成交付物。\n")
		builder.WriteString("原始任务：")
		builder.WriteString(strings.TrimSpace(task))

		if summary := taskdomain.DescribeOutputContract(contract); summary != "未检测到显式格式约束。" {
			builder.WriteString("\n显式交付约束：")
			builder.WriteString(summary)
		}

		if len(violations) > 0 {
			builder.WriteString("\n必须修复的问题：")
			for _, violation := range violations {
				trimmed := strings.TrimSpace(violation)
				if trimmed == "" {
					continue
				}
				builder.WriteString("\n- ")
				builder.WriteString(trimmed)
			}
		}

		builder.WriteString("\n\n只返回修正后的最终交付物，不要添加任何前言、解释、标题或评审说明。")
		return builder.String()
	}

	builder.WriteString("Regenerate the deliverable using the requirements below.\n")
	builder.WriteString("Original task: ")
	builder.WriteString(strings.TrimSpace(task))

	if summary := taskdomain.DescribeOutputContractEnglish(contract); summary != "No explicit output contract was detected." {
		builder.WriteString("\nExplicit output contract: ")
		builder.WriteString(summary)
	}

	if len(violations) > 0 {
		builder.WriteString("\nIssues that must be fixed:")
		for _, violation := range violations {
			trimmed := strings.TrimSpace(violation)
			if trimmed == "" {
				continue
			}
			builder.WriteString("\n- ")
			builder.WriteString(trimmed)
		}
	}

	builder.WriteString("\n\nReturn only the corrected final deliverable. Do not add a preface, explanation, heading, or review note.")
	return builder.String()
}

func (a *ReviewerAgent) repairReviewDecision(ctx context.Context, task, taskID string, contract taskdomain.OutputContract, deliverable, review string, iteration any, chinese bool) (bool, bool) {
	prompt := buildDecisionRepairPrompt(task, contract, deliverable, review, chinese)
	output, err := a.llmSkill.Execute(ctx, map[string]any{
		"prompt":     prompt,
		"agent_name": fmt.Sprintf("%s:decision-repair", a.name),
		"task_id":    taskID,
		"iteration":  iteration,
	})
	if err != nil {
		return false, false
	}

	repaired, _ := output["output"].(string)
	return parseReviewDecision(repaired)
}

func buildDecisionRepairPrompt(task string, contract taskdomain.OutputContract, deliverable, review string, chinese bool) string {
	if chinese {
		return fmt.Sprintf(`请只输出一行结果：
Decision: [PASS] 或 Decision: [FAIL]

规则：
1. 如果交付物满足任务与显式交付约束，则输出 PASS。
2. 如果交付物缺失重要内容、跑题或违反约束，则输出 FAIL。
3. 不要输出 Thought、Feedback、解释或其他内容。

任务：%s
显式交付约束：%s
交付物：
%s

已有评审内容：
%s`, task, taskdomain.DescribeOutputContract(contract), deliverable, review)
	}

	return fmt.Sprintf(`Output exactly one line:
Decision: [PASS] or Decision: [FAIL]

Rules:
1. Return PASS only if the deliverable satisfies the task and the explicit output contract.
2. Return FAIL if anything important is missing, off-topic, or non-compliant.
3. Do not output Thought, Feedback, explanations, or any other text.

Task: %s
Explicit output contract: %s
Deliverable:
%s

Existing review:
%s`, task, taskdomain.DescribeOutputContractEnglish(contract), deliverable, review)
}

func inferReviewDecision(review string) (bool, bool) {
	normalized := strings.ToLower(strings.TrimSpace(review))
	if normalized == "" {
		return false, false
	}

	negativeCues := []string{
		"fail", "missing", "lack", "lacks", "incorrect", "wrong", "off-topic", "issue", "problem",
		"needs", "need to", "should", "revise", "improve", "fix", "tighten", "does not", "doesn't",
		"not compliant", "violates", "violation", "不满足", "缺少", "问题", "需要", "修复", "未通过", "不通过",
	}
	for _, cue := range negativeCues {
		if strings.Contains(normalized, cue) {
			return false, true
		}
	}

	positiveCues := []string{
		"pass", "looks good", "acceptable", "approved", "ship it", "satisfies", "meets", "good to go",
		"通过", "满足", "符合", "可以发布", "可发布",
	}
	for _, cue := range positiveCues {
		if strings.Contains(normalized, cue) {
			return true, true
		}
	}

	return false, false
}

var _ Agent = (*ReviewerAgent)(nil)

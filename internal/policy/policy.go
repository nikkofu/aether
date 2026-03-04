package policy

import (
	"context"
	"strings"
)

// Decision 定义了策略评估后的决策类型。
type Decision string

const (
	// DecisionAllow 允许操作。
	DecisionAllow Decision = "ALLOW"
	// DecisionDeny 拒绝操作。
	DecisionDeny Decision = "DENY"
	// DecisionRequireApproval 需要人工审批。
	DecisionRequireApproval Decision = "REQUIRE_APPROVAL"
)

// EvaluationContext 包含了策略评估所需的所有上下文信息。
type EvaluationContext struct {
	// NodeID 触发评估的节点 ID。
	NodeID string `json:"node_id"`
	// Skill 调用的技能名称。
	Skill string `json:"skill"`
	// Input 传递给该技能的输入参数。
	Input map[string]any `json:"input"`
	// Metadata 可扩展的额外元数据（如用户信息、环境信息等）。
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Policy 定义了 Aether 系统的安全与合规评估接口。
type Policy interface {
	// Evaluate 根据给定的上下文执行策略检查并返回决策。
	Evaluate(ctx context.Context, evalCtx EvaluationContext) (Decision, error)
}

// DefaultPolicy 实现了 Aether 默认的安全评估策略。
type DefaultPolicy struct{}

// NewDefaultPolicy 创建并返回一个新的 DefaultPolicy 实例。
func NewDefaultPolicy() *DefaultPolicy {
	return &DefaultPolicy{}
}

// Evaluate 执行默认的策略检查逻辑。
// 规则：
// 1. 默认允许 "llm" 技能。
// 2. 包含 "shell" 的技能名将被拒绝 (Deny)。
// 3. 包含 "git" 的技能名将需要审批 (RequireApproval)。
func (p *DefaultPolicy) Evaluate(ctx context.Context, evalCtx EvaluationContext) (Decision, error) {
	skillLower := strings.ToLower(evalCtx.Skill)

	// 规则 2: Shell 注入风险高，默认拒绝
	if strings.Contains(skillLower, "shell") {
		return DecisionDeny, nil
	}

	// 规则 3: Git 操作涉及代码库变更，需要审批
	if strings.Contains(skillLower, "git") {
		return DecisionRequireApproval, nil
	}

	// 规则 1: 默认允许 llm 相关技能（假设其受 LLM 安全网关保护）
	// 或者如果没有任何特定限制，默认设为 Allow (或根据业务需求设为 Deny)
	if skillLower == "llm" || strings.HasPrefix(skillLower, "llm-") {
		return DecisionAllow, nil
	}

	// 兜底逻辑：对于未定义的技能，默认允许（或者出于极高安全性考虑可以设为 Deny）
	return DecisionAllow, nil
}

// 确保 DefaultPolicy 实现了 Policy 接口。
var _ Policy = (*DefaultPolicy)(nil)

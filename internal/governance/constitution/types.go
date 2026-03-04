package constitution

import (
	"fmt"
	"time"
)

// ConstitutionalRule 定义了系统的元规则（宪法条文）。
// 它是系统运行的最底层约束，通常由开发者设定或通过极高难度的治理投票修改。
type ConstitutionalRule struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	// Immutable 如果为 true，则该规则不可被普通的提案 (Proposal) 修改。
	Immutable   bool      `json:"immutable"`
	CreatedAt   time.Time `json:"created_at"`
}

// Constitution 定义了管理元规则及校验系统变更的核心接口。
type Constitution interface {
	// AddRule 向宪法中注入一条新规则。
	AddRule(rule ConstitutionalRule) error
	// GetRule 检索特定条文。
	GetRule(id string) (*ConstitutionalRule, error)
	// ListRules 列出所有当前生效的元规则。
	ListRules() ([]ConstitutionalRule, error)
	// ValidatePolicyChange 根据宪法精神校验一个策略变更是否合法。
	// 它是防止 Agent 群体进化出危险行为的最后一道防火墙。
	ValidatePolicyChange(policyType string, newValue any) error
}

// ErrRuleViolation 当变更违反宪法时返回此错误。
type ErrRuleViolation struct {
	RuleID  string
	Message string
}

func (e *ErrRuleViolation) Error() string {
	return fmt.Sprintf("宪法违规 [规则: %s]: %s", e.RuleID, e.Message)
}

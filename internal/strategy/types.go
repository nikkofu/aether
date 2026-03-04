package strategy

import (
	"time"
)

// Strategy 定义了代理的执行策略和自适应优化参数。
type Strategy struct {
	AgentName   string    `json:"agent_name"`
	PromptHint  string    `json:"prompt_hint"`
	RetryLimit  int       `json:"retry_limit"`
	RoutingHint string    `json:"routing_hint"`
	MaxDuration time.Duration `json:"max_duration"`
	BudgetLimit float64   `json:"budget_limit"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StrategyStore 定义了策略的存储接口。
type StrategyStore interface {
	Get(agentName string) (*Strategy, error)
	Save(s *Strategy) error
}

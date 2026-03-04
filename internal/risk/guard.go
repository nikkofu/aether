package risk

import (
	"context"
	"fmt"

	"github.com/nikkofu/aether/internal/economy"
)

// RiskGuard 定义了系统的风险熔断阈值。
type RiskGuard struct {
	MaxTokenSupply     float64
	MaxReputationSkew  float64 // 单个 Agent 最大信誉占比 (如 0.4)
	MaxFailedTasksRate float64
	ledger             economy.Ledger
}

// NewRiskGuard 创建一个新的风险守卫。
func NewRiskGuard(l economy.Ledger, maxSupply, maxSkew, maxFail float64) *RiskGuard {
	return &RiskGuard{
		ledger:             l,
		MaxTokenSupply:     maxSupply,
		MaxReputationSkew:  maxSkew,
		MaxFailedTasksRate: maxFail,
	}
}

// CheckSystemHealth 检查全系统风险状态。如果返回 error，则应触发熔断。
func (g *RiskGuard) CheckSystemHealth(ctx context.Context, orgID string) error {
	if g.ledger == nil { return nil }

	// 1. 检查代币供应量
	accounts, _ := g.ledger.TopAgentsByReputation(ctx, orgID, 100)
	var totalBalance, totalRep float64
	var maxRep float64

	for _, acc := range accounts {
		totalBalance += acc.Balance
		totalRep += acc.Reputation
		if acc.Reputation > maxRep {
			maxRep = acc.Reputation
		}
	}

	if totalBalance > g.MaxTokenSupply {
		return fmt.Errorf("风险熔断: 代币供应总量 (%v) 超过安全阈值 (%v)", totalBalance, g.MaxTokenSupply)
	}

	// 2. 检查信誉集中度 (防止单代理垄断)
	if totalRep > 0 {
		skew := maxRep / totalRep
		if skew > g.MaxReputationSkew {
			return fmt.Errorf("风险熔断: 信誉集中度过高 (%v)，可能存在单代理垄断风险", skew)
		}
	}

	return nil
}

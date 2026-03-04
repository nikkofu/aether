package economy

import (
	"context"
	"time"
)

type Account struct {
	AgentID    string    `json:"agent_id"`
	OrgID      string    `json:"org_id"`
	Balance    float64   `json:"balance"`
	Reputation float64   `json:"reputation"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Transaction struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    float64   `json:"amount"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

type Ledger interface {
	GetAccount(ctx context.Context, orgID string, agentID string) (*Account, error)
	UpdateBalance(ctx context.Context, orgID string, agentID string, delta float64, repDelta float64) error
	AddTransaction(ctx context.Context, tx Transaction) error
	TopAgentsByReputation(ctx context.Context, orgID string, limit int) ([]Account, error)
	ApplyReputationDecay(ctx context.Context, orgID string, rate float64) error
	BurnExcessTokens(ctx context.Context, orgID string, maxTotalSupply float64) error
	ListTransactions(ctx context.Context, orgID string) ([]Transaction, error)
}

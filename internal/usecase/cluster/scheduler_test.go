package cluster

import (
	"context"
	"testing"

	"github.com/nikkofu/aether/internal/domain/economy"
	"github.com/nikkofu/aether/internal/domain/risk"
	"github.com/nikkofu/aether/pkg/logging"
)

type schedulerTestLogger struct{}

func (l *schedulerTestLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (l *schedulerTestLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (l *schedulerTestLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (l *schedulerTestLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (l *schedulerTestLogger) Sync() error                                                    { return nil }

type schedulerLedgerStub struct {
	topAgents []economy.Account
	account   *economy.Account
}

func (s *schedulerLedgerStub) GetAccount(ctx context.Context, orgID string, agentID string) (*economy.Account, error) {
	if s.account != nil {
		return s.account, nil
	}
	return nil, nil
}

func (s *schedulerLedgerStub) UpdateBalance(ctx context.Context, orgID string, agentID string, delta float64, repDelta float64) error {
	return nil
}

func (s *schedulerLedgerStub) AddTransaction(ctx context.Context, tx economy.Transaction) error {
	return nil
}

func (s *schedulerLedgerStub) TopAgentsByReputation(ctx context.Context, orgID string, limit int) ([]economy.Account, error) {
	return append([]economy.Account(nil), s.topAgents...), nil
}

func (s *schedulerLedgerStub) ApplyReputationDecay(ctx context.Context, orgID string, rate float64) error {
	return nil
}

func (s *schedulerLedgerStub) BurnExcessTokens(ctx context.Context, orgID string, maxTotalSupply float64) error {
	return nil
}

func (s *schedulerLedgerStub) ListTransactions(ctx context.Context, orgID string) ([]economy.Transaction, error) {
	return nil, nil
}

func TestSelectWorkerFallsBackToLocalWhenNoRemoteWorkers(t *testing.T) {
	scheduler := NewScheduler(&schedulerTestLogger{}, nil, nil)

	selected := scheduler.SelectWorker(context.Background(), "operational")
	if selected != "local" {
		t.Fatalf("expected local fallback, got %q", selected)
	}
}

func TestSelectWorkerFallsBackToLocalWhenRiskGuardTrips(t *testing.T) {
	ledger := &schedulerLedgerStub{
		topAgents: []economy.Account{
			{AgentID: "worker-a", Reputation: 10},
		},
	}
	scheduler := NewScheduler(
		&schedulerTestLogger{},
		ledger,
		risk.NewRiskGuard(ledger, 1000, 0.4, 1),
	)
	scheduler.RegisterHeartbeat("operational", "worker-a")

	selected := scheduler.SelectWorker(context.Background(), "operational")
	if selected != "local" {
		t.Fatalf("expected risk-guard fallback to local, got %q", selected)
	}
}

func TestSelectWorkerUsesRoundRobinWhenLedgerHasNoCandidateData(t *testing.T) {
	ledger := &schedulerLedgerStub{}
	scheduler := NewScheduler(&schedulerTestLogger{}, ledger, nil)
	scheduler.RegisterHeartbeat("operational", "worker-a")
	scheduler.RegisterHeartbeat("operational", "worker-b")

	first := scheduler.SelectWorker(context.Background(), "operational")
	second := scheduler.SelectWorker(context.Background(), "operational")
	if first != "worker-a" || second != "worker-b" {
		t.Fatalf("expected round-robin fallback, got first=%q second=%q", first, second)
	}
}

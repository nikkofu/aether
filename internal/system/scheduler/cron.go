package scheduler

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/economy"
	"github.com/nikkofu/aether/internal/logging"
)

type SystemScheduler struct {
	ledger economy.Ledger
	logger logging.Logger
}

func NewSystemScheduler(l economy.Ledger, log logging.Logger) *SystemScheduler {
	return &SystemScheduler{ledger: l, logger: log}
}

func (s *SystemScheduler) Start(ctx context.Context) {
	decayTicker := time.NewTicker(24 * time.Hour)
	defer decayTicker.Stop()
	burnTicker := time.NewTicker(24 * time.Hour)
	defer burnTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-decayTicker.C:
			s.runReputationDecay(ctx, "default")
		case <-burnTicker.C:
			s.runTokenBurn(ctx, "default")
		}
	}
}

func (s *SystemScheduler) runReputationDecay(ctx context.Context, orgID string) {
	if s.ledger == nil { return }
	const defaultDecayRate = 0.02
	err := s.ledger.ApplyReputationDecay(ctx, orgID, defaultDecayRate)
	if s.logger != nil {
		if err != nil {
			s.logger.Error(ctx, "执行信誉衰减失败", logging.Err(err), logging.String("org_id", orgID))
		} else {
			s.logger.Info(ctx, "每日信誉衰减成功", logging.Float64("rate", defaultDecayRate), logging.String("org_id", orgID))
		}
	}
}

func (s *SystemScheduler) runTokenBurn(ctx context.Context, orgID string) {
	if s.ledger == nil { return }
	const maxTotalSupply = 100000.0
	err := s.ledger.BurnExcessTokens(ctx, orgID, maxTotalSupply)
	if s.logger != nil {
		if err != nil {
			s.logger.Error(ctx, "执行通缩销毁失败", logging.Err(err), logging.String("org_id", orgID))
		} else {
			s.logger.Info(ctx, "每日通缩销毁成功", logging.Float64("max_supply", maxTotalSupply), logging.String("org_id", orgID))
		}
	}
}

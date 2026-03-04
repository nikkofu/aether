package org

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/usecase/reflection"
)

// GovernanceAgent 负责合规审计与系统完整性治理。
type GovernanceAgent struct {
	agent.BaseAgent
	subs []string
}

func NewGovernanceAgent(id string) *GovernanceAgent {
	return &GovernanceAgent{
		BaseAgent: *agent.NewBaseAgent(id, "governance"),
	}
}

func (a *GovernanceAgent) ID() string           { return a.Name() }
func (a *GovernanceAgent) Level() OrgLevel      { return LevelGovernance }
func (a *GovernanceAgent) Supervisor() string    { return "vision_board" }
func (a *GovernanceAgent) Subordinates() []string { return a.subs }

func (a *GovernanceAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		if msg.Type != "reflection.report" {
			return nil, nil
		}

		ref, ok := msg.Payload["reflection"].(*reflection.Reflection)
		if !ok {
			return nil, nil
		}

		// 1. 判断是否异常 (执行失败)
		if !ref.Success {
			return []agent.Message{{
				From:      a.Name(),
				To:        "vision_board",
				Type:      "governance.alert",
				Timestamp: time.Now(),
				Payload: map[string]any{
					"message": "严重执行故障: " + ref.ErrorMessage,
					"source":  ref.AgentName,
				},
			}}, nil
		}

		return nil, nil
	})
}

var _ OrgAgent = (*GovernanceAgent)(nil)

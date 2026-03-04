package org

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/agent"
	"github.com/nikkofu/aether/internal/logging"
	"github.com/nikkofu/aether/internal/strategy/strategic"
)

// VisionBoardAgent 代表愿景委员会。
type VisionBoardAgent struct {
	agent.BaseAgent
	supervisor string
	subs       []string
	planner    strategic.StrategicPlanner
}

func NewVisionBoardAgent(id string, planner strategic.StrategicPlanner, logger logging.Logger) *VisionBoardAgent {
	ba := agent.NewBaseAgent(id, "vision-board")
	return &VisionBoardAgent{
		BaseAgent: *ba,
		planner:   planner,
	}
}

func (a *VisionBoardAgent) ID() string           { return a.Name() }
func (a *VisionBoardAgent) Level() OrgLevel      { return LevelVision }
func (a *VisionBoardAgent) Supervisor() string    { return a.supervisor }
func (a *VisionBoardAgent) Subordinates() []string { return a.subs }

func (a *VisionBoardAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		switch msg.Type {
		case "vision.create":
			title, _ := msg.Payload["title"].(string)
			desc, _ := msg.Payload["description"].(string)

			v, err := a.planner.CreateVision(ctx, title, desc)
			if err != nil { return nil, err }

			goals, err := a.planner.PlanGoals(ctx, *v)
			if err != nil { return nil, err }

			var msgs []agent.Message
			for _, g := range goals {
				msgs = append(msgs, agent.Message{
					From: a.Name(), To: "strategic_director", Type: "goal.assigned",
					Timestamp: time.Now(), Payload: map[string]any{"goal": g},
				})
			}
			return msgs, nil

		case "governance.alert":
			return nil, nil
		}
		return nil, nil
	})
}

var _ OrgAgent = (*VisionBoardAgent)(nil)

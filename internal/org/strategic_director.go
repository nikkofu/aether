package org

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/agent"
	"github.com/nikkofu/aether/internal/logging"
	"github.com/nikkofu/aether/internal/strategy/strategic"
)

// StrategicDirectorAgent 负责战略落实。
type StrategicDirectorAgent struct {
	agent.BaseAgent
	supervisor string
	subs       []string
	planner    strategic.StrategicPlanner
}

func NewStrategicDirectorAgent(id string, planner strategic.StrategicPlanner, logger logging.Logger) *StrategicDirectorAgent {
	ba := agent.NewBaseAgent(id, "strategic-director")
	return &StrategicDirectorAgent{
		BaseAgent: *ba,
		planner:   planner,
	}
}

func (a *StrategicDirectorAgent) ID() string           { return a.Name() }
func (a *StrategicDirectorAgent) Level() OrgLevel      { return LevelStrategic }
func (a *StrategicDirectorAgent) Supervisor() string    { return a.supervisor }
func (a *StrategicDirectorAgent) Subordinates() []string { return a.subs }

func (a *StrategicDirectorAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		switch msg.Type {
		case "goal.assigned":
			goalMap, _ := msg.Payload["goal"].(map[string]any)
			goal := strategic.Goal{
				ID:          goalMap["id"].(string),
				Title:       goalMap["title"].(string),
				Description: goalMap["description"].(string),
			}

			milestones, err := a.planner.PlanMilestones(ctx, goal)
			if err != nil { return nil, err }

			var msgs []agent.Message
			for _, m := range milestones {
				msgs = append(msgs, agent.Message{
					From: a.Name(), To: "tactical_manager", Type: "milestone.assigned",
					Timestamp: time.Now(), Payload: map[string]any{"milestone": m, "goal_id": goal.ID},
				})
			}
			return msgs, nil

		case "milestone.feedback":
			success, _ := msg.Payload["success"].(bool)
			if !success {
				goalID, _ := msg.Payload["goal_id"].(string)
				feedback, _ := msg.Payload["feedback"].(string)
				newMilestones, err := a.planner.Replan(ctx, strategic.Goal{ID: goalID}, feedback)
				if err != nil { return nil, err }

				var msgs []agent.Message
				for _, nm := range newMilestones {
					msgs = append(msgs, agent.Message{
						From: a.Name(), To: "tactical_manager", Type: "milestone.assigned",
						Timestamp: time.Now(), Payload: map[string]any{"milestone": nm, "goal_id": goalID},
					})
				}
				return msgs, nil
			}
		}
		return nil, nil
	})
}

var _ OrgAgent = (*StrategicDirectorAgent)(nil)

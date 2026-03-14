package org

import (
	"context"
	"strings"
	"testing"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/domain/strategy/strategic"
)

type strategicPlannerStub struct {
	milestones []strategic.Milestone
	replanned  []strategic.Milestone
}

func (s *strategicPlannerStub) CreateVision(ctx context.Context, title, desc string) (*strategic.Vision, error) {
	return &strategic.Vision{Title: title, Description: desc}, nil
}

func (s *strategicPlannerStub) PlanGoals(ctx context.Context, vision strategic.Vision) ([]strategic.Goal, error) {
	return nil, nil
}

func (s *strategicPlannerStub) PlanMilestones(ctx context.Context, goal strategic.Goal) ([]strategic.Milestone, error) {
	return s.milestones, nil
}

func (s *strategicPlannerStub) Replan(ctx context.Context, goal strategic.Goal, feedback string) ([]strategic.Milestone, error) {
	return s.replanned, nil
}

func TestStrategicDirectorDelegatesGoalIntoMilestones(t *testing.T) {
	director := NewStrategicDirectorAgent("strategic_director", &strategicPlannerStub{
		milestones: []strategic.Milestone{
			{ID: "ms-1", GoalID: "goal-1", Title: "Define the boundary"},
			{ID: "ms-2", GoalID: "goal-1", Title: "Ship the delivery path"},
		},
	}, nil)

	messages, err := director.Handle(context.Background(), agentdomain.Message{
		ID:   "task-1",
		From: "workflow.hierarchical",
		To:   director.Name(),
		Type: "goal.assigned",
		Payload: map[string]any{
			"task_id":         "task-1",
			"trace_id":        "trace-1",
			"org_id":          "org-1",
			"delivery_target": "workflow.hierarchical",
			"delivery_type":   agentdomain.TypeGoalResult,
			"goal": map[string]any{
				"id":          "goal-1",
				"title":       "Refactor the hierarchy",
				"description": "Refactor the hierarchy",
			},
		},
	})
	if err != nil {
		t.Fatalf("goal assignment failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 milestone assignments, got %#v", messages)
	}
	for _, msg := range messages {
		if msg.To != "tactical_manager" || msg.Type != "milestone.assigned" {
			t.Fatalf("unexpected milestone assignment: %#v", msg)
		}
		if msg.Payload["delivery_target"] != director.Name() {
			t.Fatalf("expected milestone to return to strategic director, got %#v", msg.Payload["delivery_target"])
		}
	}
}

func TestStrategicDirectorAggregatesMilestoneResults(t *testing.T) {
	director := NewStrategicDirectorAgent("strategic_director", &strategicPlannerStub{
		milestones: []strategic.Milestone{
			{ID: "ms-1", GoalID: "goal-1", Title: "Define the boundary"},
			{ID: "ms-2", GoalID: "goal-1", Title: "Ship the delivery path"},
		},
	}, nil)

	_, err := director.Handle(context.Background(), agentdomain.Message{
		ID:   "task-2",
		From: "workflow.hierarchical",
		To:   director.Name(),
		Type: "goal.assigned",
		Payload: map[string]any{
			"task_id":         "task-2",
			"trace_id":        "trace-2",
			"org_id":          "org-1",
			"delivery_target": "workflow.hierarchical",
			"delivery_type":   agentdomain.TypeGoalResult,
			"goal": map[string]any{
				"id":          "goal-1",
				"title":       "Refactor the hierarchy",
				"description": "Refactor the hierarchy",
			},
		},
	})
	if err != nil {
		t.Fatalf("goal assignment failed: %v", err)
	}

	messages, err := director.Handle(context.Background(), agentdomain.Message{
		ID:   "task-2",
		From: "tactical_manager",
		To:   director.Name(),
		Type: "milestone.feedback",
		Payload: map[string]any{
			"task_id":      "task-2",
			"goal_id":      "goal-1",
			"milestone_id": "ms-2",
			"success":      true,
			"output":       "delivery path is explicit",
		},
	})
	if err != nil {
		t.Fatalf("first feedback failed: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected director to wait for all milestones, got %#v", messages)
	}

	messages, err = director.Handle(context.Background(), agentdomain.Message{
		ID:   "task-2",
		From: "tactical_manager",
		To:   director.Name(),
		Type: "milestone.feedback",
		Payload: map[string]any{
			"task_id":      "task-2",
			"goal_id":      "goal-1",
			"milestone_id": "ms-1",
			"success":      true,
			"output":       "boundary is explicit",
		},
	})
	if err != nil {
		t.Fatalf("second feedback failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected a single goal result, got %#v", messages)
	}
	if messages[0].Type != agentdomain.TypeGoalResult {
		t.Fatalf("expected goal.result, got %s", messages[0].Type)
	}
	if messages[0].To != "workflow.hierarchical" {
		t.Fatalf("expected result to return to workflow.hierarchical, got %s", messages[0].To)
	}
	output, _ := messages[0].Payload["output"].(string)
	if output == "" {
		t.Fatalf("expected aggregated output, got %#v", messages[0].Payload["output"])
	}
	if !strings.Contains(output, "Milestone 1 - Define the boundary") || !strings.Contains(output, "Milestone 2 - Ship the delivery path") {
		t.Fatalf("expected ordered milestone sections, got %q", output)
	}
	if strings.Index(output, "Milestone 1 - Define the boundary") > strings.Index(output, "Milestone 2 - Ship the delivery path") {
		t.Fatalf("expected goal output to preserve milestone planning order, got %q", output)
	}
}

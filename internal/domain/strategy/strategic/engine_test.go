package strategic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nikkofu/aether/pkg/logging"
)

type testStrategicLogger struct{}

func (l *testStrategicLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (l *testStrategicLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (l *testStrategicLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (l *testStrategicLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (l *testStrategicLogger) Sync() error                                                    { return nil }

type strategicStoreStub struct {
	milestoneUpdates map[string]string
}

func (s *strategicStoreStub) SaveVision(*Vision) error         { return nil }
func (s *strategicStoreStub) SaveGoals([]Goal) error           { return nil }
func (s *strategicStoreStub) SaveMilestones([]Milestone) error { return nil }
func (s *strategicStoreStub) ListActiveGoals() ([]Goal, error) { return nil, nil }
func (s *strategicStoreStub) GetMilestones(goalID string) ([]Milestone, error) {
	return nil, nil
}
func (s *strategicStoreStub) UpdateGoalStatus(id string, status string) error { return nil }
func (s *strategicStoreStub) UpdateMilestoneStatus(id string, status string) error {
	if s.milestoneUpdates == nil {
		s.milestoneUpdates = make(map[string]string)
	}
	s.milestoneUpdates[id] = status
	return nil
}

type launcherStub struct {
	reqs []TaskLaunchRequest
	err  error
}

func (l *launcherStub) LaunchTask(ctx context.Context, req TaskLaunchRequest) error {
	l.reqs = append(l.reqs, req)
	return l.err
}

func TestEngineExecuteMilestoneLaunchesStandardTask(t *testing.T) {
	store := &strategicStoreStub{}
	launcher := &launcherStub{}
	engine := NewEngine(nil, store, &testStrategicLogger{}, launcher)

	goal := Goal{ID: "goal-1", OrgID: "org-1", Title: "Scale delivery"}
	ms := &Milestone{ID: "ms-1", GoalID: goal.ID, Title: "Refactor workflow", CreatedAt: time.Now()}

	engine.executeMilestone(context.Background(), goal, ms)

	if len(launcher.reqs) != 1 {
		t.Fatalf("expected one task launch, got %d", len(launcher.reqs))
	}
	req := launcher.reqs[0]
	if req.WorkflowPattern != "coordinator" {
		t.Fatalf("expected coordinator pattern, got %s", req.WorkflowPattern)
	}
	if req.OrgID != goal.OrgID {
		t.Fatalf("expected org %s, got %s", goal.OrgID, req.OrgID)
	}
	if req.Input["goal_id"] != goal.ID {
		t.Fatalf("expected goal_id=%s, got %#v", goal.ID, req.Input["goal_id"])
	}
	if req.Input["milestone_id"] != ms.ID {
		t.Fatalf("expected milestone_id=%s, got %#v", ms.ID, req.Input["milestone_id"])
	}
	if store.milestoneUpdates[ms.ID] != "active" {
		t.Fatalf("expected milestone to become active, got %#v", store.milestoneUpdates)
	}
}

func TestEngineExecuteMilestoneKeepsPendingWhenLaunchFails(t *testing.T) {
	store := &strategicStoreStub{}
	launcher := &launcherStub{err: errors.New("submit failed")}
	engine := NewEngine(nil, store, &testStrategicLogger{}, launcher)

	goal := Goal{ID: "goal-2", OrgID: "org-1", Title: "Scale delivery"}
	ms := &Milestone{ID: "ms-2", GoalID: goal.ID, Title: "Refactor workflow", CreatedAt: time.Now()}

	engine.executeMilestone(context.Background(), goal, ms)

	if len(launcher.reqs) != 1 {
		t.Fatalf("expected one launch attempt, got %d", len(launcher.reqs))
	}
	if _, ok := store.milestoneUpdates[ms.ID]; ok {
		t.Fatalf("expected milestone to remain pending when launch fails, got %#v", store.milestoneUpdates)
	}
}

package agent

import (
	"context"
	"testing"
)

func TestCoordinatorWorkflowAgentDelegatesToTacticalManager(t *testing.T) {
	workflow := NewCoordinatorWorkflowAgent(CoordinatorWorkflowAgentName)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowCoordinatorStart,
		Payload: map[string]any{
			"task_id":      "task-1",
			"description":  "Coordinate a refactor",
			"trace_id":     "trace-1",
			"goal_id":      "goal-1",
			"milestone_id": "ms-1",
			"org_id":       "org-1",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(startMessages) != 1 || startMessages[0].To != "tactical_manager" || startMessages[0].Type != "milestone.assigned" {
		t.Fatalf("unexpected start response: %#v", startMessages)
	}
	if startMessages[0].Payload["delivery_type"] != TypeCoordinationResult {
		t.Fatalf("expected coordination delivery type, got %#v", startMessages[0].Payload["delivery_type"])
	}

	finalMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "tactical_manager",
		To:   workflow.Name(),
		Type: TypeCoordinationResult,
		Payload: map[string]any{
			"task_id": "task-1",
			"success": true,
			"output":  "coordinated result",
			"goal_id": "goal-1",
			"ms_id":   "ms-1",
		},
	})
	if err != nil {
		t.Fatalf("coordination success handling failed: %v", err)
	}
	if len(finalMessages) != 1 || finalMessages[0].To != "cli" || finalMessages[0].Type != "final_report" {
		t.Fatalf("unexpected final response: %#v", finalMessages)
	}
}

func TestCoordinatorWorkflowAgentFailsOnRejectedCoordination(t *testing.T) {
	workflow := NewCoordinatorWorkflowAgent(CoordinatorWorkflowAgentName)

	_, err := workflow.Handle(context.Background(), Message{
		ID:   "task-2",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowCoordinatorStart,
		Payload: map[string]any{
			"task_id":     "task-2",
			"description": "Coordinate a refactor",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	messages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-2",
		From: "tactical_manager",
		To:   workflow.Name(),
		Type: TypeCoordinationResult,
		Payload: map[string]any{
			"task_id":  "task-2",
			"success":  false,
			"feedback": "worker pool failed",
		},
	})
	if err != nil {
		t.Fatalf("coordination failure handling failed: %v", err)
	}
	if len(messages) != 1 || messages[0].Type != TypeSystemAlert || messages[0].To != "supervisor" {
		t.Fatalf("unexpected failure response: %#v", messages)
	}
}

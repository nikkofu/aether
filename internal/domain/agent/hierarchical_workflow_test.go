package agent

import (
	"context"
	"testing"
)

func TestHierarchicalWorkflowAgentDelegatesToStrategicDirector(t *testing.T) {
	workflow := NewHierarchicalWorkflowAgent(HierarchicalWorkflowAgentName)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-h1",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowHierarchicalStart,
		Payload: map[string]any{
			"task_id":     "task-h1",
			"description": "Deliver a multi-milestone refactor",
			"trace_id":    "trace-h1",
			"goal_id":     "goal-h1",
			"org_id":      "org-h1",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(startMessages) != 1 {
		t.Fatalf("expected 1 start message, got %#v", startMessages)
	}
	if startMessages[0].To != "strategic_director" || startMessages[0].Type != "goal.assigned" {
		t.Fatalf("unexpected start message: %#v", startMessages[0])
	}
	if startMessages[0].Payload["delivery_type"] != TypeGoalResult {
		t.Fatalf("expected goal.result delivery type, got %#v", startMessages[0].Payload["delivery_type"])
	}

	finalMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-h1",
		From: "strategic_director",
		To:   workflow.Name(),
		Type: TypeGoalResult,
		Payload: map[string]any{
			"task_id": "task-h1",
			"success": true,
			"output":  "goal delivered",
		},
	})
	if err != nil {
		t.Fatalf("goal result handling failed: %v", err)
	}
	if len(finalMessages) != 1 || finalMessages[0].To != "cli" || finalMessages[0].Type != "final_report" {
		t.Fatalf("unexpected final response: %#v", finalMessages)
	}
}

func TestHierarchicalWorkflowAgentFailsOnRejectedGoal(t *testing.T) {
	workflow := NewHierarchicalWorkflowAgent(HierarchicalWorkflowAgentName)

	_, err := workflow.Handle(context.Background(), Message{
		ID:   "task-h2",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowHierarchicalStart,
		Payload: map[string]any{
			"task_id":     "task-h2",
			"description": "Deliver a multi-milestone refactor",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	messages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-h2",
		From: "strategic_director",
		To:   workflow.Name(),
		Type: TypeGoalResult,
		Payload: map[string]any{
			"task_id":  "task-h2",
			"success":  false,
			"feedback": "milestone delivery failed",
		},
	})
	if err != nil {
		t.Fatalf("goal failure handling failed: %v", err)
	}
	if len(messages) != 1 || messages[0].Type != TypeSystemAlert || messages[0].To != "supervisor" {
		t.Fatalf("unexpected failure response: %#v", messages)
	}
}

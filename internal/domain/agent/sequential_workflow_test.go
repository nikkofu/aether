package agent

import (
	"context"
	"testing"
)

func TestSequentialWorkflowAgentRoutesPlannerCoderReviewerFlow(t *testing.T) {
	workflow := NewSequentialWorkflowAgent(SequentialWorkflowAgentName)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowSequentialStart,
		Payload: map[string]any{
			"task_id":     "task-1",
			"description": "Refactor the task pipeline",
			"trace_id":    "trace-1",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(startMessages) != 1 || startMessages[0].To != "planner" || startMessages[0].Type != "task_plan_request" {
		t.Fatalf("unexpected start response: %#v", startMessages)
	}
	if startMessages[0].Payload["delivery_type"] != TypePlanGenerated {
		t.Fatalf("expected plan delivery type, got %#v", startMessages[0].Payload["delivery_type"])
	}

	planMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "planner",
		To:   workflow.Name(),
		Type: TypePlanGenerated,
		Payload: map[string]any{
			"task_id": "task-1",
			"task":    "Refactor the task pipeline",
			"plan":    "Create an explicit sequential workflow agent",
		},
	})
	if err != nil {
		t.Fatalf("plan handling failed: %v", err)
	}
	if len(planMessages) != 1 || planMessages[0].To != "coder" || planMessages[0].Type != "instruction" {
		t.Fatalf("unexpected plan response: %#v", planMessages)
	}
	if planMessages[0].Payload["delivery_type"] != TypeDraftGenerated {
		t.Fatalf("expected draft delivery type, got %#v", planMessages[0].Payload["delivery_type"])
	}

	draftMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "coder",
		To:   workflow.Name(),
		Type: TypeDraftGenerated,
		Payload: map[string]any{
			"task_id": "task-1",
			"task":    "Refactor the task pipeline",
			"code":    "draft-v1",
		},
	})
	if err != nil {
		t.Fatalf("draft handling failed: %v", err)
	}
	if len(draftMessages) != 1 || draftMessages[0].To != "reviewer" || draftMessages[0].Type != "review_request" {
		t.Fatalf("unexpected draft response: %#v", draftMessages)
	}

	finalMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "reviewer",
		To:   workflow.Name(),
		Type: "review_result",
		Payload: map[string]any{
			"task_id":  "task-1",
			"approved": true,
			"code":     "draft-v1",
		},
	})
	if err != nil {
		t.Fatalf("review success handling failed: %v", err)
	}
	if len(finalMessages) != 1 || finalMessages[0].To != "cli" || finalMessages[0].Type != "final_report" {
		t.Fatalf("unexpected final response: %#v", finalMessages)
	}
}

func TestSequentialWorkflowAgentFailsOnRejectedReview(t *testing.T) {
	workflow := NewSequentialWorkflowAgent(SequentialWorkflowAgentName)

	_, err := workflow.Handle(context.Background(), Message{
		ID:   "task-2",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowSequentialStart,
		Payload: map[string]any{
			"task_id":     "task-2",
			"description": "Refactor the task pipeline",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	messages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-2",
		From: "reviewer",
		To:   workflow.Name(),
		Type: "review_result",
		Payload: map[string]any{
			"task_id":  "task-2",
			"approved": false,
			"feedback": "review rejected this draft",
			"code":     "draft-v1",
		},
	})
	if err != nil {
		t.Fatalf("review failure handling failed: %v", err)
	}
	if len(messages) != 1 || messages[0].Type != TypeSystemAlert || messages[0].To != "supervisor" {
		t.Fatalf("unexpected failure response: %#v", messages)
	}
}

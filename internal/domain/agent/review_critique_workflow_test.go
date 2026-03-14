package agent

import (
	"context"
	"testing"
)

func TestReviewCritiqueWorkflowAgentLoopsUntilApproval(t *testing.T) {
	workflow := NewReviewCritiqueWorkflowAgent(ReviewCritiqueWorkflowAgentName)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowReviewCritiqueStart,
		Payload: map[string]any{
			"task_id":        "task-1",
			"description":    "Refactor the task pipeline",
			"trace_id":       "trace-1",
			"max_iterations": 2,
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(startMessages) != 1 || startMessages[0].To != "coder" || startMessages[0].Type != "instruction" {
		t.Fatalf("unexpected start response: %#v", startMessages)
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

	retryMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "reviewer",
		To:   workflow.Name(),
		Type: "review_result",
		Payload: map[string]any{
			"task_id":  "task-1",
			"approved": false,
			"feedback": "Need clearer retry handling",
			"code":     "draft-v1",
		},
	})
	if err != nil {
		t.Fatalf("review failure handling failed: %v", err)
	}
	if len(retryMessages) != 1 || retryMessages[0].To != "coder" || retryMessages[0].Type != "instruction" {
		t.Fatalf("unexpected retry response: %#v", retryMessages)
	}
	if retryMessages[0].Payload["delivery_type"] != TypeDraftGenerated {
		t.Fatalf("expected retry instruction to preserve draft delivery type, got %#v", retryMessages[0].Payload["delivery_type"])
	}

	finalMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-1",
		From: "reviewer",
		To:   workflow.Name(),
		Type: "review_result",
		Payload: map[string]any{
			"task_id":  "task-1",
			"approved": true,
			"code":     "draft-v2",
		},
	})
	if err != nil {
		t.Fatalf("review success handling failed: %v", err)
	}
	if len(finalMessages) != 1 || finalMessages[0].To != "cli" || finalMessages[0].Type != "final_report" {
		t.Fatalf("unexpected final response: %#v", finalMessages)
	}
}

func TestReviewCritiqueWorkflowAgentFailsAfterMaxIterations(t *testing.T) {
	workflow := NewReviewCritiqueWorkflowAgent(ReviewCritiqueWorkflowAgentName)

	_, err := workflow.Handle(context.Background(), Message{
		ID:   "task-2",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowReviewCritiqueStart,
		Payload: map[string]any{
			"task_id":        "task-2",
			"description":    "Refactor the task pipeline",
			"max_iterations": 1,
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
			"feedback": "still failing",
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

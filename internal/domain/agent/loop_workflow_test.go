package agent

import (
	"context"
	"testing"
)

func TestLoopWorkflowAgentLoopsUntilApproval(t *testing.T) {
	workflow := NewLoopWorkflowAgent(LoopWorkflowAgentName)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-loop-1",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowLoopStart,
		Payload: map[string]any{
			"task_id":        "task-loop-1",
			"description":    "Keep refining until the loop exits",
			"trace_id":       "trace-loop-1",
			"max_iterations": 2,
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(startMessages) != 1 || startMessages[0].To != "coder" || startMessages[0].Type != "instruction" {
		t.Fatalf("unexpected start response: %#v", startMessages)
	}

	retryMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-loop-1",
		From: "reviewer",
		To:   workflow.Name(),
		Type: "review_result",
		Payload: map[string]any{
			"task_id":  "task-loop-1",
			"approved": false,
			"feedback": "continue the loop",
			"code":     "draft-v1",
		},
	})
	if err != nil {
		t.Fatalf("review failure handling failed: %v", err)
	}
	if len(retryMessages) != 1 || retryMessages[0].To != "coder" || retryMessages[0].Type != "instruction" {
		t.Fatalf("unexpected retry response: %#v", retryMessages)
	}

	finalMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-loop-1",
		From: "reviewer",
		To:   workflow.Name(),
		Type: "review_result",
		Payload: map[string]any{
			"task_id":  "task-loop-1",
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

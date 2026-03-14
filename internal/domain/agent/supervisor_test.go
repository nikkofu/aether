package agent

import (
	"context"
	"testing"

	"github.com/nikkofu/aether/pkg/logging"
)

type testSupervisorLogger struct{}

func (l *testSupervisorLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (l *testSupervisorLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (l *testSupervisorLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (l *testSupervisorLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (l *testSupervisorLogger) Sync() error                                                    { return nil }

func TestSupervisorAgentDoesNotOrchestrateWorkflowMessages(t *testing.T) {
	supervisor := NewSupervisorAgent("supervisor", nil, &testSupervisorLogger{})

	tests := []Message{
		{
			ID:   "task-1",
			From: "legacy",
			To:   supervisor.Name(),
			Type: "task",
			Payload: map[string]any{
				"task_id": "task-1",
			},
		},
		{
			ID:   "task-1",
			From: "planner",
			To:   supervisor.Name(),
			Type: "instruction",
			Payload: map[string]any{
				"task_id": "task-1",
			},
		},
		{
			ID:   "task-1",
			From: "reviewer",
			To:   supervisor.Name(),
			Type: "review_result",
			Payload: map[string]any{
				"task_id":  "task-1",
				"approved": true,
			},
		},
	}

	for _, msg := range tests {
		responses, err := supervisor.Handle(context.Background(), msg)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", msg.Type, err)
		}
		if len(responses) != 0 {
			t.Fatalf("expected no orchestration response for %s, got %#v", msg.Type, responses)
		}
	}
}

func TestSupervisorAgentDoesNotRetryOnCriticalAlerts(t *testing.T) {
	supervisor := NewSupervisorAgent("supervisor", nil, &testSupervisorLogger{})

	responses, err := supervisor.Handle(context.Background(), Message{
		ID:   "task-2",
		From: "workflow.sequential",
		To:   supervisor.Name(),
		Type: TypeSystemAlert,
		Payload: map[string]any{
			"severity": "CRITICAL",
			"message":  "workflow crashed",
			"task_id":  "task-2",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(responses) != 0 {
		t.Fatalf("expected critical alert to stay observational, got %#v", responses)
	}
}

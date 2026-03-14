package agent

import (
	"context"
	"strings"
	"testing"
)

type parallelWorkflowTestManager struct {
	next int
}

func (m *parallelWorkflowTestManager) Register(a Agent) {}

func (m *parallelWorkflowTestManager) RegisterRole(role string, factory AgentRoleFactory) {}

func (m *parallelWorkflowTestManager) Spawn(ctx context.Context, role string, payload map[string]any) (Agent, error) {
	m.next++
	return &parallelWorkflowTestAgent{
		BaseAgent: *NewBaseAgent(branchDisplayName(role, m.next), role),
	}, nil
}

func (m *parallelWorkflowTestManager) List() []Agent { return nil }

func (m *parallelWorkflowTestManager) Get(name string) (Agent, bool) { return nil, false }

func (m *parallelWorkflowTestManager) GetStats() ManagerStats { return ManagerStats{} }

type parallelWorkflowTestAgent struct {
	BaseAgent
}

func (a *parallelWorkflowTestAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	return nil, nil
}

func TestParallelWorkflowAgentFansOutAndAggregatesResults(t *testing.T) {
	manager := &parallelWorkflowTestManager{}
	workflow := NewParallelWorkflowAgent(ParallelWorkflowAgentName, manager)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-1",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowParallelStart,
		Payload: map[string]any{
			"task_id":     "task-parallel-1",
			"description": "Ship the new workflow",
			"trace_id":    "trace-parallel-1",
			"org_id":      "org-1",
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(startMessages) != 3 {
		t.Fatalf("expected 3 branch assignments, got %d", len(startMessages))
	}
	if startMessages[0].Type != "task.assigned" || startMessages[0].Payload["subtask_index"] != 1 {
		t.Fatalf("unexpected first branch assignment: %#v", startMessages[0])
	}

	for index, output := range []string{"analysis output", "implementation output"} {
		messages, err := workflow.Handle(context.Background(), Message{
			ID:   "task-parallel-1",
			From: "operational",
			To:   workflow.Name(),
			Type: "task.completed",
			Payload: map[string]any{
				"task_id":       "task-parallel-1",
				"success":       true,
				"output":        output,
				"subtask_index": index + 1,
			},
		})
		if err != nil {
			t.Fatalf("branch %d completion failed: %v", index+1, err)
		}
		if len(messages) != 0 {
			t.Fatalf("expected no final message before all branches complete, got %#v", messages)
		}
	}

	finalMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-1",
		From: "operational",
		To:   workflow.Name(),
		Type: "task.completed",
		Payload: map[string]any{
			"task_id":       "task-parallel-1",
			"success":       true,
			"output":        "verification output",
			"subtask_index": 3,
		},
	})
	if err != nil {
		t.Fatalf("final branch completion failed: %v", err)
	}
	if len(finalMessages) != 1 || finalMessages[0].To != "cli" || finalMessages[0].Type != "final_report" {
		t.Fatalf("unexpected final response: %#v", finalMessages)
	}

	result, _ := finalMessages[0].Payload["result"].(string)
	if !strings.Contains(result, "[1/3] Analysis") || !strings.Contains(result, "[2/3] Implementation") || !strings.Contains(result, "[3/3] Verification") {
		t.Fatalf("expected ordered parallel synthesis, got %q", result)
	}
}

func TestParallelWorkflowAgentPreservesNamedBranches(t *testing.T) {
	manager := &parallelWorkflowTestManager{}
	workflow := NewParallelWorkflowAgent(ParallelWorkflowAgentName, manager)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-named",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowParallelStart,
		Payload: map[string]any{
			"task_id":     "task-parallel-named",
			"description": "Use named branches",
			"trace_id":    "trace-parallel-named",
			"parallel_branches": []any{
				map[string]any{"name": "Plan", "task": "Analyze current architecture"},
				map[string]any{"name": "Build", "task": "Implement the workflow change"},
			},
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if startMessages[0].Payload["branch_name"] != "Plan" || startMessages[1].Payload["branch_name"] != "Build" {
		t.Fatalf("expected named branches in fan-out payloads, got %#v", startMessages)
	}

	messages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-named",
		From: "operational",
		To:   workflow.Name(),
		Type: "task.completed",
		Payload: map[string]any{
			"task_id":       "task-parallel-named",
			"success":       true,
			"output":        "plan output",
			"subtask_index": 1,
			"branch_name":   "Plan",
		},
	})
	if err != nil {
		t.Fatalf("first completion failed: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no final message yet, got %#v", messages)
	}

	finalMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-named",
		From: "operational",
		To:   workflow.Name(),
		Type: "task.completed",
		Payload: map[string]any{
			"task_id":       "task-parallel-named",
			"success":       true,
			"output":        "build output",
			"subtask_index": 2,
			"branch_name":   "Build",
		},
	})
	if err != nil {
		t.Fatalf("second completion failed: %v", err)
	}

	result, _ := finalMessages[0].Payload["result"].(string)
	if !strings.Contains(result, "[1/2] Plan") || !strings.Contains(result, "[2/2] Build") {
		t.Fatalf("expected named branches in final synthesis, got %q", result)
	}
}

func TestParallelWorkflowAgentPreservesNamedBranchesFromTypedMaps(t *testing.T) {
	manager := &parallelWorkflowTestManager{}
	workflow := NewParallelWorkflowAgent(ParallelWorkflowAgentName, manager)

	startMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-typed",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowParallelStart,
		Payload: map[string]any{
			"task_id":     "task-parallel-typed",
			"description": "Use typed map branches",
			"parallel_branches": []map[string]any{
				{"name": "Plan", "task": "Analyze current architecture"},
				{"name": "Build", "task": "Implement the workflow change"},
			},
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if startMessages[0].Payload["branch_name"] != "Plan" || startMessages[1].Payload["branch_name"] != "Build" {
		t.Fatalf("expected typed map branch names in fan-out payloads, got %#v", startMessages)
	}
}

func TestParallelWorkflowAgentAggregatesFailures(t *testing.T) {
	manager := &parallelWorkflowTestManager{}
	workflow := NewParallelWorkflowAgent(ParallelWorkflowAgentName, manager)

	_, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-2",
		From: "api",
		To:   workflow.Name(),
		Type: TypeWorkflowParallelStart,
		Payload: map[string]any{
			"task_id":           "task-parallel-2",
			"description":       "Use custom branches",
			"parallel_branches": []any{"branch one", "branch two"},
		},
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	messages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-2",
		From: "operational",
		To:   workflow.Name(),
		Type: "task.completed",
		Payload: map[string]any{
			"task_id":       "task-parallel-2",
			"success":       true,
			"output":        "branch one result",
			"subtask_index": 1,
		},
	})
	if err != nil {
		t.Fatalf("first branch completion failed: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected no terminal message yet, got %#v", messages)
	}

	failureMessages, err := workflow.Handle(context.Background(), Message{
		ID:   "task-parallel-2",
		From: "operational",
		To:   workflow.Name(),
		Type: "task.completed",
		Payload: map[string]any{
			"task_id":       "task-parallel-2",
			"success":       false,
			"feedback":      "verification failed",
			"subtask_index": 2,
		},
	})
	if err != nil {
		t.Fatalf("failure branch completion failed: %v", err)
	}
	if len(failureMessages) != 1 || failureMessages[0].To != "supervisor" || failureMessages[0].Type != TypeSystemAlert {
		t.Fatalf("unexpected failure response: %#v", failureMessages)
	}
}

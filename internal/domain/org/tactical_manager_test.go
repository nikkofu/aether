package org

import (
	"context"
	"strings"
	"testing"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/domain/capability"
)

type tacticalLLMStub struct {
	output          string
	synthesisOutput string
	lastPrompt      string
}

func (l *tacticalLLMStub) Name() string { return "llm" }

func (l *tacticalLLMStub) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	prompt, _ := input["prompt"].(string)
	l.lastPrompt = prompt
	if strings.Contains(prompt, "请整合以下子任务输出") && l.synthesisOutput != "" {
		return map[string]any{"output": l.synthesisOutput}, nil
	}
	return map[string]any{"output": l.output}, nil
}

type tacticalAgentManagerStub struct {
	spawned []string
}

func (m *tacticalAgentManagerStub) Register(a agentdomain.Agent) {}

func (m *tacticalAgentManagerStub) RegisterRole(role string, factory agentdomain.AgentRoleFactory) {}

func (m *tacticalAgentManagerStub) Spawn(ctx context.Context, role string, payload map[string]any) (agentdomain.Agent, error) {
	name := role + "-" + string(rune('a'+len(m.spawned)))
	m.spawned = append(m.spawned, name)
	return &tacticalSpawnedAgent{BaseAgent: agentdomain.NewBaseAgent(name, role)}, nil
}

func (m *tacticalAgentManagerStub) List() []agentdomain.Agent { return nil }

func (m *tacticalAgentManagerStub) Get(name string) (agentdomain.Agent, bool) { return nil, false }

func (m *tacticalAgentManagerStub) GetStats() agentdomain.ManagerStats {
	return agentdomain.ManagerStats{}
}

type tacticalSpawnedAgent struct {
	*agentdomain.BaseAgent
}

func (a *tacticalSpawnedAgent) Handle(ctx context.Context, msg agentdomain.Message) ([]agentdomain.Message, error) {
	return nil, nil
}

var _ capability.Capability = (*tacticalLLMStub)(nil)

func TestTacticalManagerFansOutMilestoneIntoMultipleWorkerTasks(t *testing.T) {
	manager := &tacticalAgentManagerStub{}
	tactical := NewTacticalManagerAgent("tactical_manager", "strategic_director", manager, &tacticalLLMStub{
		output: `["subtask 1","subtask 2","subtask 3"]`,
	}, nil)

	messages, err := tactical.Handle(context.Background(), agentdomain.Message{
		ID:   "task-1",
		From: "workflow.coordinator",
		To:   tactical.Name(),
		Type: "milestone.assigned",
		Payload: map[string]any{
			"task_id":         "task-1",
			"goal_id":         "goal-1",
			"milestone_id":    "ms-1",
			"trace_id":        "trace-1",
			"delivery_target": "workflow.coordinator",
			"delivery_type":   agentdomain.TypeCoordinationResult,
			"milestone": map[string]any{
				"id":    "ms-1",
				"title": "Ship the coordinator workflow",
			},
		},
	})
	if err != nil {
		t.Fatalf("milestone handling failed: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 worker tasks, got %d", len(messages))
	}
	for _, msg := range messages {
		if msg.Type != "task.assigned" {
			t.Fatalf("expected task.assigned message, got %s", msg.Type)
		}
		if msg.Payload["delivery_target"] != tactical.Name() {
			t.Fatalf("expected worker tasks to return to tactical manager, got %#v", msg.Payload["delivery_target"])
		}
		if msg.Payload["delivery_type"] != "task.completed" {
			t.Fatalf("expected worker tasks to return task.completed, got %#v", msg.Payload["delivery_type"])
		}
	}
	if len(manager.spawned) != 3 {
		t.Fatalf("expected 3 spawned workers, got %d", len(manager.spawned))
	}
}

func TestTacticalManagerAggregatesWorkerResults(t *testing.T) {
	manager := &tacticalAgentManagerStub{}
	tactical := NewTacticalManagerAgent("tactical_manager", "strategic_director", manager, &tacticalLLMStub{
		output:          `["subtask 1","subtask 2"]`,
		synthesisOutput: "coordinated milestone result",
	}, nil)

	_, err := tactical.Handle(context.Background(), agentdomain.Message{
		ID:   "task-2",
		From: "workflow.coordinator",
		To:   tactical.Name(),
		Type: "milestone.assigned",
		Payload: map[string]any{
			"task_id":         "task-2",
			"goal_id":         "goal-2",
			"milestone_id":    "ms-2",
			"trace_id":        "trace-2",
			"delivery_target": "workflow.coordinator",
			"delivery_type":   agentdomain.TypeCoordinationResult,
			"milestone": map[string]any{
				"id":    "ms-2",
				"title": "Ship the coordinator workflow",
			},
		},
	})
	if err != nil {
		t.Fatalf("milestone handling failed: %v", err)
	}

	messages, err := tactical.Handle(context.Background(), agentdomain.Message{
		ID:   "task-2",
		From: "operational-a",
		To:   tactical.Name(),
		Type: "task.completed",
		Payload: map[string]any{
			"task_id":      "task-2",
			"milestone_id": "ms-2",
			"success":      true,
			"output":       "result 1",
			"trace_id":     "trace-2",
		},
	})
	if err != nil {
		t.Fatalf("first completion failed: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected aggregation to wait for all workers, got %#v", messages)
	}

	messages, err = tactical.Handle(context.Background(), agentdomain.Message{
		ID:   "task-2",
		From: "operational-b",
		To:   tactical.Name(),
		Type: "task.completed",
		Payload: map[string]any{
			"task_id":      "task-2",
			"milestone_id": "ms-2",
			"success":      true,
			"output":       "result 2",
			"trace_id":     "trace-2",
		},
	})
	if err != nil {
		t.Fatalf("second completion failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected a single aggregated result, got %#v", messages)
	}
	if messages[0].Type != agentdomain.TypeCoordinationResult {
		t.Fatalf("expected coordination.result, got %s", messages[0].Type)
	}
	if messages[0].Payload["success"] != true {
		t.Fatalf("expected aggregated success, got %#v", messages[0].Payload["success"])
	}
	if messages[0].Payload["output"] != "coordinated milestone result" {
		t.Fatalf("unexpected aggregated output: %#v", messages[0].Payload["output"])
	}
}

func TestTacticalManagerKeepsSeparateMilestoneStatesForSameTask(t *testing.T) {
	manager := &tacticalAgentManagerStub{}
	tactical := NewTacticalManagerAgent("tactical_manager", "strategic_director", manager, &tacticalLLMStub{
		output: `["subtask 1"]`,
	}, nil)

	for _, milestoneID := range []string{"ms-1", "ms-2"} {
		_, err := tactical.Handle(context.Background(), agentdomain.Message{
			ID:   "task-3",
			From: "strategic_director",
			To:   tactical.Name(),
			Type: "milestone.assigned",
			Payload: map[string]any{
				"task_id":         "task-3",
				"goal_id":         "goal-3",
				"milestone_id":    milestoneID,
				"trace_id":        "trace-3",
				"delivery_target": "strategic_director",
				"delivery_type":   "milestone.feedback",
				"milestone": map[string]any{
					"id":    milestoneID,
					"title": milestoneID,
				},
			},
		})
		if err != nil {
			t.Fatalf("milestone %s assignment failed: %v", milestoneID, err)
		}
	}

	for _, milestoneID := range []string{"ms-1", "ms-2"} {
		messages, err := tactical.Handle(context.Background(), agentdomain.Message{
			ID:   "task-3",
			From: "operational-a",
			To:   tactical.Name(),
			Type: "task.completed",
			Payload: map[string]any{
				"task_id":      "task-3",
				"milestone_id": milestoneID,
				"success":      true,
				"output":       milestoneID + " done",
				"trace_id":     "trace-3",
			},
		})
		if err != nil {
			t.Fatalf("milestone %s completion failed: %v", milestoneID, err)
		}
		if len(messages) != 1 {
			t.Fatalf("expected aggregated feedback for %s, got %#v", milestoneID, messages)
		}
		if messages[0].Payload["milestone_id"] != milestoneID {
			t.Fatalf("expected milestone_id=%s, got %#v", milestoneID, messages[0].Payload["milestone_id"])
		}
	}
}

func TestBuildMilestoneBreakdownPromptStaysTaskGeneric(t *testing.T) {
	prompt := buildMilestoneBreakdownPrompt("正式发布前验收")
	if !strings.Contains(prompt, "可以直接执行的子任务") {
		t.Fatalf("expected generic execution wording, got %q", prompt)
	}
	if strings.Contains(prompt, "Go 开发任务") {
		t.Fatalf("expected prompt to avoid hard-coded Go development wording, got %q", prompt)
	}
}

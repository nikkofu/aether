package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const HierarchicalWorkflowAgentName = "workflow.hierarchical"

type hierarchicalWorkflowState struct {
	TaskID  string
	Task    string
	TraceID string
	GoalID  string
	OrgID   string
}

type HierarchicalWorkflowAgent struct {
	BaseAgent
	mu     sync.Mutex
	states map[string]*hierarchicalWorkflowState
}

func NewHierarchicalWorkflowAgent(name string) *HierarchicalWorkflowAgent {
	if name == "" {
		name = HierarchicalWorkflowAgentName
	}

	return &HierarchicalWorkflowAgent{
		BaseAgent: *NewBaseAgent(name, "workflow-hierarchical"),
		states:    make(map[string]*hierarchicalWorkflowState),
	}
}

func (a *HierarchicalWorkflowAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		switch msg.Type {
		case TypeWorkflowHierarchicalStart:
			return a.handleStart(msg)
		case TypeGoalResult:
			return a.handleGoalResult(msg)
		default:
			return nil, nil
		}
	})
}

func (a *HierarchicalWorkflowAgent) handleStart(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	task, _ := msg.Payload["description"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("hierarchical workflow requires task_id")
	}
	if task == "" {
		return nil, fmt.Errorf("hierarchical workflow requires task description")
	}

	state := &hierarchicalWorkflowState{
		TaskID:  taskID,
		Task:    task,
		TraceID: stringValue(msg.Payload["trace_id"]),
		GoalID:  stringValue(msg.Payload["goal_id"]),
		OrgID:   stringValue(msg.Payload["org_id"]),
	}
	if state.GoalID == "" {
		state.GoalID = taskID
	}
	if state.OrgID == "" {
		state.OrgID = "default"
	}

	a.mu.Lock()
	a.states[taskID] = state
	a.mu.Unlock()

	return []Message{{
		ID:        taskID,
		From:      a.name,
		To:        "strategic_director",
		Type:      "goal.assigned",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id":         taskID,
			"trace_id":        state.TraceID,
			"org_id":          state.OrgID,
			"delivery_target": a.name,
			"delivery_type":   TypeGoalResult,
			"goal": map[string]any{
				"id":          state.GoalID,
				"org_id":      state.OrgID,
				"title":       state.Task,
				"description": state.Task,
			},
		},
	}}, nil
}

func (a *HierarchicalWorkflowAgent) handleGoalResult(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.lookupState(taskID)
	if !ok {
		return nil, fmt.Errorf("hierarchical workflow state not found for task %s", taskID)
	}

	a.deleteState(taskID)

	success, _ := msg.Payload["success"].(bool)
	if success {
		result := stringValue(msg.Payload["output"])
		if result == "" {
			result = state.Task
		}
		return []Message{{
			ID:        taskID,
			From:      a.name,
			To:        "cli",
			Type:      "final_report",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"result":           result,
				"task_id":          taskID,
				"goal_id":          state.GoalID,
				"workflow_pattern": "hierarchical",
			},
		}}, nil
	}

	message := stringValue(msg.Payload["feedback"])
	if message == "" {
		message = stringValue(msg.Payload["error"])
	}
	if message == "" {
		message = fmt.Sprintf("hierarchical workflow failed task %s", taskID)
	}

	return []Message{{
		ID:        taskID,
		From:      a.name,
		To:        "supervisor",
		Type:      TypeSystemAlert,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"severity":         "HIGH",
			"message":          message,
			"task_id":          taskID,
			"trace_id":         state.TraceID,
			"goal_id":          state.GoalID,
			"workflow_pattern": "hierarchical",
		},
	}}, nil
}

func (a *HierarchicalWorkflowAgent) lookupState(taskID string) (hierarchicalWorkflowState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[taskID]
	if !ok || state == nil {
		return hierarchicalWorkflowState{}, false
	}
	return *state, true
}

func (a *HierarchicalWorkflowAgent) deleteState(taskID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.states, taskID)
}

var _ Agent = (*HierarchicalWorkflowAgent)(nil)

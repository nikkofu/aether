package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const CoordinatorWorkflowAgentName = "workflow.coordinator"

type coordinatorWorkflowState struct {
	TaskID      string
	Task        string
	TraceID     string
	GoalID      string
	MilestoneID string
	OrgID       string
}

type CoordinatorWorkflowAgent struct {
	BaseAgent
	mu     sync.Mutex
	states map[string]*coordinatorWorkflowState
}

func NewCoordinatorWorkflowAgent(name string) *CoordinatorWorkflowAgent {
	if name == "" {
		name = CoordinatorWorkflowAgentName
	}

	return &CoordinatorWorkflowAgent{
		BaseAgent: *NewBaseAgent(name, "workflow-coordinator"),
		states:    make(map[string]*coordinatorWorkflowState),
	}
}

func (a *CoordinatorWorkflowAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		switch msg.Type {
		case TypeWorkflowCoordinatorStart:
			return a.handleStart(msg)
		case TypeCoordinationResult:
			return a.handleCoordinationResult(msg)
		default:
			return nil, nil
		}
	})
}

func (a *CoordinatorWorkflowAgent) handleStart(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	task, _ := msg.Payload["description"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("coordinator workflow requires task_id")
	}
	if task == "" {
		return nil, fmt.Errorf("coordinator workflow requires task description")
	}

	state := &coordinatorWorkflowState{
		TaskID:      taskID,
		Task:        task,
		TraceID:     stringValue(msg.Payload["trace_id"]),
		GoalID:      stringValue(msg.Payload["goal_id"]),
		MilestoneID: stringValue(msg.Payload["milestone_id"]),
		OrgID:       stringValue(msg.Payload["org_id"]),
	}
	if state.GoalID == "" {
		state.GoalID = taskID
	}
	if state.MilestoneID == "" {
		state.MilestoneID = taskID
	}
	if state.OrgID == "" {
		state.OrgID = "default"
	}

	a.mu.Lock()
	a.states[taskID] = state
	a.mu.Unlock()

	return []Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "tactical_manager",
		Type:      "milestone.assigned",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id":         taskID,
			"trace_id":        state.TraceID,
			"org_id":          state.OrgID,
			"goal_id":         state.GoalID,
			"milestone_id":    state.MilestoneID,
			"milestone":       map[string]any{"id": state.MilestoneID, "title": state.Task},
			"delivery_target": a.name,
			"delivery_type":   TypeCoordinationResult,
		},
	}}, nil
}

func (a *CoordinatorWorkflowAgent) handleCoordinationResult(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.lookupState(taskID)
	if !ok {
		return nil, fmt.Errorf("coordinator workflow state not found for task %s", taskID)
	}

	a.deleteState(taskID)

	success, _ := msg.Payload["success"].(bool)
	if success {
		result := stringValue(msg.Payload["output"])
		if result == "" {
			result = state.Task
		}
		return []Message{{
			ID:        msg.ID,
			From:      a.name,
			To:        "cli",
			Type:      "final_report",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"result":           result,
				"task_id":          taskID,
				"workflow_pattern": "coordinator",
				"goal_id":          state.GoalID,
				"milestone_id":     state.MilestoneID,
			},
		}}, nil
	}

	message := stringValue(msg.Payload["feedback"])
	if message == "" {
		message = stringValue(msg.Payload["error"])
	}
	if message == "" {
		message = fmt.Sprintf("coordinator workflow failed task %s", taskID)
	}

	return []Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "supervisor",
		Type:      TypeSystemAlert,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"severity":         "HIGH",
			"message":          message,
			"task_id":          taskID,
			"trace_id":         state.TraceID,
			"workflow_pattern": "coordinator",
			"goal_id":          state.GoalID,
			"milestone_id":     state.MilestoneID,
		},
	}}, nil
}

func (a *CoordinatorWorkflowAgent) lookupState(taskID string) (coordinatorWorkflowState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[taskID]
	if !ok || state == nil {
		return coordinatorWorkflowState{}, false
	}
	return *state, true
}

func (a *CoordinatorWorkflowAgent) deleteState(taskID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.states, taskID)
}

var _ Agent = (*CoordinatorWorkflowAgent)(nil)

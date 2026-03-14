package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const SequentialWorkflowAgentName = "workflow.sequential"

type sequentialWorkflowState struct {
	TaskID  string
	Task    string
	TraceID string
}

type SequentialWorkflowAgent struct {
	BaseAgent
	mu     sync.Mutex
	states map[string]*sequentialWorkflowState
}

func NewSequentialWorkflowAgent(name string) *SequentialWorkflowAgent {
	if name == "" {
		name = SequentialWorkflowAgentName
	}

	return &SequentialWorkflowAgent{
		BaseAgent: *NewBaseAgent(name, "workflow-sequential"),
		states:    make(map[string]*sequentialWorkflowState),
	}
}

func (a *SequentialWorkflowAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		switch msg.Type {
		case TypeWorkflowSequentialStart:
			return a.handleStart(msg)
		case TypePlanGenerated:
			return a.handlePlanGenerated(msg)
		case TypeDraftGenerated:
			return a.handleDraftGenerated(msg)
		case "review_result":
			return a.handleReviewResult(msg)
		default:
			return nil, nil
		}
	})
}

func (a *SequentialWorkflowAgent) handleStart(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	task, _ := msg.Payload["description"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("sequential workflow requires task_id")
	}
	if task == "" {
		return nil, fmt.Errorf("sequential workflow requires task description")
	}

	state := &sequentialWorkflowState{
		TaskID:  taskID,
		Task:    task,
		TraceID: stringValue(msg.Payload["trace_id"]),
	}

	a.mu.Lock()
	a.states[taskID] = state
	a.mu.Unlock()

	return []Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "planner",
		Type:      "task_plan_request",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"description":     task,
			"task_id":         taskID,
			"trace_id":        state.TraceID,
			"delivery_target": a.name,
			"delivery_type":   TypePlanGenerated,
		},
	}}, nil
}

func (a *SequentialWorkflowAgent) handlePlanGenerated(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.lookupState(taskID)
	if !ok {
		return nil, fmt.Errorf("sequential workflow state not found for task %s", taskID)
	}

	task, _ := msg.Payload["task"].(string)
	if task == "" {
		task = state.Task
	}

	return []Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "coder",
		Type:      "instruction",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"plan":            msg.Payload["plan"],
			"task":            task,
			"task_id":         taskID,
			"trace_id":        state.TraceID,
			"delivery_target": a.name,
			"delivery_type":   TypeDraftGenerated,
			"progress_target": "supervisor",
		},
	}}, nil
}

func (a *SequentialWorkflowAgent) handleDraftGenerated(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.lookupState(taskID)
	if !ok {
		return nil, fmt.Errorf("sequential workflow state not found for task %s", taskID)
	}

	task, _ := msg.Payload["task"].(string)
	if task == "" {
		task = state.Task
	}

	return []Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "reviewer",
		Type:      "review_request",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"code":     msg.Payload["code"],
			"task":     task,
			"task_id":  taskID,
			"trace_id": state.TraceID,
		},
	}}, nil
}

func (a *SequentialWorkflowAgent) handleReviewResult(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.lookupState(taskID)
	if !ok {
		return nil, fmt.Errorf("sequential workflow state not found for task %s", taskID)
	}

	a.deleteState(taskID)

	approved, _ := msg.Payload["approved"].(bool)
	if approved {
		return []Message{{
			ID:        msg.ID,
			From:      a.name,
			To:        "cli",
			Type:      "final_report",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"result":           msg.Payload["code"],
				"task_id":          taskID,
				"workflow_pattern": "sequential",
			},
		}}, nil
	}

	feedback := stringValue(msg.Payload["feedback"])
	if feedback == "" {
		feedback = fmt.Sprintf("sequential workflow review rejected task %s", taskID)
	}

	return []Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "supervisor",
		Type:      TypeSystemAlert,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"severity":         "HIGH",
			"message":          feedback,
			"task_id":          taskID,
			"trace_id":         state.TraceID,
			"workflow_pattern": "sequential",
		},
	}}, nil
}

func (a *SequentialWorkflowAgent) lookupState(taskID string) (sequentialWorkflowState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[taskID]
	if !ok || state == nil {
		return sequentialWorkflowState{}, false
	}
	return *state, true
}

func (a *SequentialWorkflowAgent) deleteState(taskID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.states, taskID)
}

var _ Agent = (*SequentialWorkflowAgent)(nil)

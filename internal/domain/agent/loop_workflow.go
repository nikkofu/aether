package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	LoopWorkflowAgentName                = "workflow.loop"
	ReviewCritiqueWorkflowAgentName      = "workflow.review_critique"
	IterativeRefinementWorkflowAgentName = "workflow.iterative_refinement"
)

type loopWorkflowDefinition struct {
	defaultName      string
	role             string
	workflowPattern  string
	startType        string
	revisionLeadText string
}

type loopWorkflowState struct {
	TaskID        string
	Task          string
	TraceID       string
	Iteration     int
	MaxIterations int
}

type LoopWorkflowAgent struct {
	BaseAgent
	definition loopWorkflowDefinition
	mu         sync.Mutex
	states     map[string]*loopWorkflowState
}

func NewLoopWorkflowAgent(name string) *LoopWorkflowAgent {
	return newConfiguredLoopWorkflowAgent(name, loopWorkflowDefinition{
		defaultName:      LoopWorkflowAgentName,
		role:             "workflow-loop",
		workflowPattern:  "loop",
		startType:        TypeWorkflowLoopStart,
		revisionLeadText: "Continue the loop using this review feedback:",
	})
}

func NewReviewCritiqueWorkflowAgent(name string) *LoopWorkflowAgent {
	return newConfiguredLoopWorkflowAgent(name, loopWorkflowDefinition{
		defaultName:      ReviewCritiqueWorkflowAgentName,
		role:             "workflow-review-critique",
		workflowPattern:  "review_critique",
		startType:        TypeWorkflowReviewCritiqueStart,
		revisionLeadText: "Revise the implementation using this review feedback:",
	})
}

func NewIterativeRefinementWorkflowAgent(name string) *LoopWorkflowAgent {
	return newConfiguredLoopWorkflowAgent(name, loopWorkflowDefinition{
		defaultName:      IterativeRefinementWorkflowAgentName,
		role:             "workflow-iterative-refinement",
		workflowPattern:  "iterative_refinement",
		startType:        TypeWorkflowIterativeStart,
		revisionLeadText: "Iteratively refine the implementation using this feedback:",
	})
}

func newConfiguredLoopWorkflowAgent(name string, definition loopWorkflowDefinition) *LoopWorkflowAgent {
	if name == "" {
		name = definition.defaultName
	}

	return &LoopWorkflowAgent{
		BaseAgent:  *NewBaseAgent(name, definition.role),
		definition: definition,
		states:     make(map[string]*loopWorkflowState),
	}
}

func (a *LoopWorkflowAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		switch msg.Type {
		case a.definition.startType:
			return a.handleStart(msg)
		case TypeDraftGenerated:
			return a.handleDraftGenerated(msg)
		case "review_result":
			return a.handleReviewResult(msg)
		default:
			return nil, nil
		}
	})
}

func (a *LoopWorkflowAgent) handleStart(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	task, _ := msg.Payload["description"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("%s workflow requires task_id", a.definition.workflowPattern)
	}
	if task == "" {
		return nil, fmt.Errorf("%s workflow requires task description", a.definition.workflowPattern)
	}

	state := &loopWorkflowState{
		TaskID:        taskID,
		Task:          task,
		TraceID:       stringValue(msg.Payload["trace_id"]),
		Iteration:     1,
		MaxIterations: parsePositiveInt(msg.Payload["max_iterations"], 3),
	}

	a.mu.Lock()
	a.states[taskID] = state
	a.mu.Unlock()

	return []Message{a.instructionMessage(msg.ID, state, "", "")}, nil
}

func (a *LoopWorkflowAgent) handleDraftGenerated(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.lookupState(taskID)
	if !ok {
		return nil, fmt.Errorf("%s state not found for task %s", a.definition.workflowPattern, taskID)
	}

	code, _ := msg.Payload["code"].(string)
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
			"code":      code,
			"task":      task,
			"task_id":   taskID,
			"trace_id":  state.TraceID,
			"iteration": state.Iteration,
		},
	}}, nil
}

func (a *LoopWorkflowAgent) handleReviewResult(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.lookupState(taskID)
	if !ok {
		return nil, fmt.Errorf("%s state not found for task %s", a.definition.workflowPattern, taskID)
	}

	approved, _ := msg.Payload["approved"].(bool)
	if approved {
		a.deleteState(taskID)
		return []Message{{
			ID:        msg.ID,
			From:      a.name,
			To:        "cli",
			Type:      "final_report",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"result":           msg.Payload["code"],
				"task_id":          taskID,
				"workflow_pattern": a.definition.workflowPattern,
			},
		}}, nil
	}

	if state.Iteration >= state.MaxIterations {
		a.deleteState(taskID)
		return []Message{{
			ID:        msg.ID,
			From:      a.name,
			To:        "supervisor",
			Type:      TypeSystemAlert,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"severity":         "HIGH",
				"message":          fmt.Sprintf("%s exceeded max iterations (%d)", a.definition.workflowPattern, state.MaxIterations),
				"task_id":          taskID,
				"workflow_pattern": a.definition.workflowPattern,
			},
		}}, nil
	}

	feedback, _ := msg.Payload["revision_feedback"].(string)
	if feedback == "" {
		feedback, _ = msg.Payload["feedback"].(string)
	}
	code, _ := msg.Payload["code"].(string)
	nextState, ok := a.advanceIteration(taskID)
	if !ok {
		return nil, fmt.Errorf("%s state not found for task %s", a.definition.workflowPattern, taskID)
	}
	return []Message{a.instructionMessage(msg.ID, &nextState, feedback, code)}, nil
}

func (a *LoopWorkflowAgent) instructionMessage(messageID string, state *loopWorkflowState, feedback, previousCode string) Message {
	plan := state.Task
	if feedback != "" {
		plan = fmt.Sprintf("%s\n%s\n\nPrevious draft:\n%s", a.definition.revisionLeadText, feedback, previousCode)
	}

	return Message{
		ID:        messageID,
		From:      a.name,
		To:        "coder",
		Type:      "instruction",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task":            state.Task,
			"plan":            plan,
			"task_id":         state.TaskID,
			"trace_id":        state.TraceID,
			"iteration":       state.Iteration,
			"delivery_target": a.name,
			"delivery_type":   TypeDraftGenerated,
			"progress_target": "supervisor",
		},
	}
}

func (a *LoopWorkflowAgent) lookupState(taskID string) (loopWorkflowState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[taskID]
	if !ok || state == nil {
		return loopWorkflowState{}, false
	}
	return *state, true
}

func (a *LoopWorkflowAgent) advanceIteration(taskID string) (loopWorkflowState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[taskID]
	if !ok || state == nil {
		return loopWorkflowState{}, false
	}
	state.Iteration++
	return *state, true
}

func (a *LoopWorkflowAgent) deleteState(taskID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.states, taskID)
}

func parsePositiveInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed >= 1 {
			return int(typed)
		}
	case float32:
		if typed >= 1 {
			return int(typed)
		}
	}

	if fallback > 0 {
		return fallback
	}
	return 1
}

func stringValue(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

var _ Agent = (*LoopWorkflowAgent)(nil)

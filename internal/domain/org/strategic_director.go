package org

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/domain/strategy/strategic"
	"github.com/nikkofu/aether/pkg/logging"
)

// StrategicDirectorAgent 负责战略落实。
type StrategicDirectorAgent struct {
	agent.BaseAgent
	supervisor string
	subs       []string
	planner    strategic.StrategicPlanner
	mu         sync.Mutex
	states     map[string]*strategicGoalState
}

type strategicGoalState struct {
	TaskID         string
	Goal           strategic.Goal
	TraceID        string
	OrgID          string
	DeliveryTarget string
	DeliveryType   string
	Pending        int
	ReplanCount    int
	MilestoneOrder []string
	Milestones     map[string]string
	Outputs        []goalMilestoneOutput
	Errors         []goalMilestoneError
}

type goalMilestoneOutput struct {
	MilestoneID string
	Title       string
	Output      string
}

type goalMilestoneError struct {
	MilestoneID string
	Title       string
	Message     string
}

func NewStrategicDirectorAgent(id string, planner strategic.StrategicPlanner, logger logging.Logger) *StrategicDirectorAgent {
	ba := agent.NewBaseAgent(id, "strategic-director")
	return &StrategicDirectorAgent{
		BaseAgent: *ba,
		planner:   planner,
		states:    make(map[string]*strategicGoalState),
	}
}

func (a *StrategicDirectorAgent) ID() string             { return a.Name() }
func (a *StrategicDirectorAgent) Level() OrgLevel        { return LevelStrategic }
func (a *StrategicDirectorAgent) Supervisor() string     { return a.supervisor }
func (a *StrategicDirectorAgent) Subordinates() []string { return a.subs }

func (a *StrategicDirectorAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		switch msg.Type {
		case "goal.assigned":
			return a.handleGoalAssigned(ctx, msg)
		case "milestone.feedback":
			return a.handleMilestoneFeedback(ctx, msg)
		default:
			return nil, nil
		}
	})
}

func (a *StrategicDirectorAgent) handleGoalAssigned(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	if a.planner == nil {
		return nil, fmt.Errorf("strategic director planner is not initialized")
	}

	goal, err := goalFromPayload(msg.Payload["goal"])
	if err != nil {
		return nil, err
	}

	taskID := firstNonEmptyString(msg.Payload["task_id"], msg.ID, goal.ID)
	orgID := firstNonEmptyString(goal.OrgID, msg.Payload["org_id"], "default")
	traceID := firstNonEmptyString(msg.Payload["trace_id"])
	deliveryTarget := firstNonEmptyString(msg.Payload["delivery_target"])
	deliveryType := firstNonEmptyString(msg.Payload["delivery_type"], agent.TypeGoalResult)

	goal.OrgID = orgID
	milestones, err := a.planner.PlanMilestones(ctx, goal)
	if err != nil {
		return nil, err
	}
	if len(milestones) == 0 {
		return nil, fmt.Errorf("strategic director planner returned no milestones for goal %s", goal.ID)
	}

	state := &strategicGoalState{
		TaskID:         taskID,
		Goal:           goal,
		TraceID:        traceID,
		OrgID:          orgID,
		DeliveryTarget: deliveryTarget,
		DeliveryType:   deliveryType,
		Pending:        len(milestones),
		MilestoneOrder: make([]string, 0, len(milestones)),
		Milestones:     make(map[string]string, len(milestones)),
		Outputs:        make([]goalMilestoneOutput, 0, len(milestones)),
		Errors:         make([]goalMilestoneError, 0, len(milestones)),
	}
	a.storeState(state)

	return a.buildMilestoneAssignments(taskID, milestones)
}

func (a *StrategicDirectorAgent) handleMilestoneFeedback(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	taskID := firstNonEmptyString(msg.Payload["task_id"], msg.ID)
	state, ok := a.recordMilestoneFeedback(taskID, msg)
	if !ok {
		return nil, fmt.Errorf("strategic director state not found for task %s", taskID)
	}
	if state.Pending > 0 {
		return nil, nil
	}

	if len(state.Errors) > 0 {
		if state.ReplanCount < 1 && a.planner != nil {
			replanned, err := a.planner.Replan(ctx, state.Goal, formatGoalErrors(state.MilestoneOrder, state.Errors))
			if err == nil && len(replanned) > 0 {
				return a.resetStateForReplan(taskID, replanned)
			}
		}

		a.deleteState(taskID)
		if state.DeliveryTarget == "" {
			return nil, nil
		}

		return []agent.Message{{
			ID:        taskID,
			From:      a.Name(),
			To:        state.DeliveryTarget,
			Type:      state.DeliveryType,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"success":  false,
				"goal_id":  state.Goal.ID,
				"task_id":  state.TaskID,
				"trace_id": state.TraceID,
				"feedback": formatGoalErrors(state.MilestoneOrder, state.Errors),
				"error":    formatGoalErrors(state.MilestoneOrder, state.Errors),
			},
		}}, nil
	}

	a.deleteState(taskID)
	if state.DeliveryTarget == "" {
		return nil, nil
	}

	return []agent.Message{{
		ID:        taskID,
		From:      a.Name(),
		To:        state.DeliveryTarget,
		Type:      state.DeliveryType,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"success":  true,
			"goal_id":  state.Goal.ID,
			"task_id":  state.TaskID,
			"trace_id": state.TraceID,
			"output":   renderGoalOutputs(state.Goal, state.MilestoneOrder, state.Outputs),
		},
	}}, nil
}

func (a *StrategicDirectorAgent) buildMilestoneAssignments(taskID string, milestones []strategic.Milestone) ([]agent.Message, error) {
	a.mu.Lock()
	state, ok := a.states[taskID]
	if !ok || state == nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("strategic director state not found for task %s", taskID)
	}
	state.Milestones = make(map[string]string, len(milestones))
	state.MilestoneOrder = state.MilestoneOrder[:0]
	for _, milestone := range milestones {
		state.Milestones[milestone.ID] = milestone.Title
		state.MilestoneOrder = append(state.MilestoneOrder, milestone.ID)
	}
	a.mu.Unlock()

	messages := make([]agent.Message, 0, len(milestones))
	for _, milestone := range milestones {
		messages = append(messages, agent.Message{
			ID:        taskID,
			From:      a.Name(),
			To:        "tactical_manager",
			Type:      "milestone.assigned",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"task_id":         taskID,
				"goal_id":         state.Goal.ID,
				"milestone_id":    milestone.ID,
				"trace_id":        state.TraceID,
				"org_id":          state.OrgID,
				"delivery_target": a.Name(),
				"delivery_type":   "milestone.feedback",
				"milestone": map[string]any{
					"id":    milestone.ID,
					"title": milestone.Title,
				},
			},
		})
	}

	return messages, nil
}

func (a *StrategicDirectorAgent) storeState(state *strategicGoalState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.states[state.TaskID] = state
}

func (a *StrategicDirectorAgent) recordMilestoneFeedback(taskID string, msg agent.Message) (strategicGoalState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[taskID]
	if !ok || state == nil {
		return strategicGoalState{}, false
	}

	milestoneID := firstNonEmptyString(msg.Payload["milestone_id"], msg.Payload["ms_id"])
	title := state.Milestones[milestoneID]
	if title == "" {
		title = milestoneID
	}

	if output, ok := msg.Payload["output"].(string); ok && strings.TrimSpace(output) != "" {
		state.Outputs = append(state.Outputs, goalMilestoneOutput{
			MilestoneID: milestoneID,
			Title:       title,
			Output:      strings.TrimSpace(output),
		})
	}

	success, _ := msg.Payload["success"].(bool)
	if !success {
		message := firstNonEmptyString(msg.Payload["feedback"], msg.Payload["error"])
		if message == "" {
			message = fmt.Sprintf("milestone %s failed", milestoneID)
		}
		state.Errors = append(state.Errors, goalMilestoneError{
			MilestoneID: milestoneID,
			Title:       title,
			Message:     message,
		})
	}

	if state.Pending > 0 {
		state.Pending--
	}

	return *state, true
}

func (a *StrategicDirectorAgent) resetStateForReplan(taskID string, milestones []strategic.Milestone) ([]agent.Message, error) {
	a.mu.Lock()
	state, ok := a.states[taskID]
	if !ok || state == nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("strategic director state not found for task %s", taskID)
	}
	state.Pending = len(milestones)
	state.ReplanCount++
	state.Outputs = make([]goalMilestoneOutput, 0, len(milestones))
	state.Errors = make([]goalMilestoneError, 0, len(milestones))
	a.mu.Unlock()

	return a.buildMilestoneAssignments(taskID, milestones)
}

func (a *StrategicDirectorAgent) deleteState(taskID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.states, taskID)
}

func goalFromPayload(value any) (strategic.Goal, error) {
	switch typed := value.(type) {
	case strategic.Goal:
		if typed.ID == "" {
			return strategic.Goal{}, fmt.Errorf("strategic director requires goal id")
		}
		if typed.Title == "" && typed.Description != "" {
			typed.Title = typed.Description
		}
		if typed.Description == "" {
			typed.Description = typed.Title
		}
		return typed, nil
	case map[string]any:
		goal := strategic.Goal{
			ID:          firstNonEmptyString(typed["id"]),
			OrgID:       firstNonEmptyString(typed["org_id"]),
			VisionID:    firstNonEmptyString(typed["vision_id"]),
			Title:       firstNonEmptyString(typed["title"]),
			Description: firstNonEmptyString(typed["description"]),
			Status:      firstNonEmptyString(typed["status"]),
		}
		if goal.ID == "" {
			return strategic.Goal{}, fmt.Errorf("strategic director requires goal id")
		}
		if goal.Title == "" && goal.Description != "" {
			goal.Title = goal.Description
		}
		if goal.Description == "" {
			goal.Description = goal.Title
		}
		return goal, nil
	default:
		return strategic.Goal{}, fmt.Errorf("strategic director requires goal payload")
	}
}

func formatGoalOutputs(goal strategic.Goal, outputs []goalMilestoneOutput) string {
	return renderGoalOutputs(goal, nil, outputs)
}

func renderGoalOutputs(goal strategic.Goal, milestoneOrder []string, outputs []goalMilestoneOutput) string {
	var builder strings.Builder
	orderedOutputs := orderedGoalOutputs(milestoneOrder, outputs)

	if goal.Title != "" {
		builder.WriteString("Goal: ")
		builder.WriteString(goal.Title)
		builder.WriteString("\n\n")
	}

	for index, output := range orderedOutputs {
		if index > 0 {
			builder.WriteString("\n\n")
		}
		title := strings.TrimSpace(output.Title)
		if title == "" {
			title = output.MilestoneID
		}
		builder.WriteString("Milestone ")
		builder.WriteString(fmt.Sprintf("%d", index+1))
		if title != "" {
			builder.WriteString(" - ")
			builder.WriteString(title)
		}
		builder.WriteString("\n")
		builder.WriteString(output.Output)
	}

	return strings.TrimSpace(builder.String())
}

func formatGoalErrors(milestoneOrder []string, errors []goalMilestoneError) string {
	if len(errors) == 0 {
		return ""
	}

	orderedErrors := orderedGoalErrors(milestoneOrder, errors)
	parts := make([]string, 0, len(orderedErrors))
	for _, item := range orderedErrors {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = item.MilestoneID
		}
		if title != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", title, item.Message))
			continue
		}
		parts = append(parts, item.Message)
	}

	return strings.Join(parts, "\n")
}

func orderedGoalOutputs(milestoneOrder []string, outputs []goalMilestoneOutput) []goalMilestoneOutput {
	if len(outputs) <= 1 || len(milestoneOrder) == 0 {
		return append([]goalMilestoneOutput(nil), outputs...)
	}

	orderIndex := make(map[string]int, len(milestoneOrder))
	for index, milestoneID := range milestoneOrder {
		orderIndex[milestoneID] = index
	}

	ordered := append([]goalMilestoneOutput(nil), outputs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return orderIndex[ordered[i].MilestoneID] < orderIndex[ordered[j].MilestoneID]
	})
	return ordered
}

func orderedGoalErrors(milestoneOrder []string, errors []goalMilestoneError) []goalMilestoneError {
	if len(errors) <= 1 || len(milestoneOrder) == 0 {
		return append([]goalMilestoneError(nil), errors...)
	}

	orderIndex := make(map[string]int, len(milestoneOrder))
	for index, milestoneID := range milestoneOrder {
		orderIndex[milestoneID] = index
	}

	ordered := append([]goalMilestoneError(nil), errors...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return orderIndex[ordered[i].MilestoneID] < orderIndex[ordered[j].MilestoneID]
	})
	return ordered
}

var _ OrgAgent = (*StrategicDirectorAgent)(nil)

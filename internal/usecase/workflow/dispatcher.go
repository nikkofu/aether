package workflow

import (
	"context"
	"fmt"
	"time"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	"github.com/nikkofu/aether/pkg/bus"
)

type LaunchInput struct {
	Source string
	OrgID  string
}

type Executor interface {
	Pattern() taskdomain.WorkflowPattern
	Execute(ctx context.Context, task *taskdomain.Task, input LaunchInput) error
}

type Dispatcher struct {
	executors map[taskdomain.WorkflowPattern]Executor
}

func NewDispatcher(executors ...Executor) *Dispatcher {
	dispatcher := &Dispatcher{
		executors: make(map[taskdomain.WorkflowPattern]Executor, len(executors)),
	}

	for _, executor := range executors {
		if executor == nil {
			continue
		}
		dispatcher.executors[executor.Pattern()] = executor
	}

	return dispatcher
}

func (d *Dispatcher) Supports(pattern taskdomain.WorkflowPattern) bool {
	if d == nil {
		return false
	}
	_, ok := d.executors[pattern]
	return ok
}

func (d *Dispatcher) Execute(ctx context.Context, task *taskdomain.Task, input LaunchInput) error {
	if d == nil {
		return fmt.Errorf("workflow dispatcher is not configured")
	}
	if task == nil {
		return fmt.Errorf("task is required")
	}

	executor, ok := d.executors[task.WorkflowPattern]
	if !ok {
		return fmt.Errorf("workflow pattern not implemented: %s", task.WorkflowPattern)
	}

	return executor.Execute(ctx, task, input)
}

type SequentialExecutor struct {
	bus bus.Bus
}

type ParallelExecutor struct {
	bus bus.Bus
}

func NewSequentialExecutor(b bus.Bus) *SequentialExecutor {
	return &SequentialExecutor{bus: b}
}

func NewParallelExecutor(b bus.Bus) *ParallelExecutor {
	return &ParallelExecutor{bus: b}
}

func (e *SequentialExecutor) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternSequential
}

func (e *ParallelExecutor) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternParallel
}

func (e *SequentialExecutor) Execute(ctx context.Context, task *taskdomain.Task, input LaunchInput) error {
	if e == nil || e.bus == nil {
		return fmt.Errorf("workflow executor requires a message bus")
	}
	if task == nil {
		return fmt.Errorf("task is required")
	}

	source := input.Source
	if source == "" {
		source = "task_service"
	}

	orgID := input.OrgID
	if orgID == "" {
		orgID = "default"
	}

	e.bus.Publish(ctx, agentdomain.Message{
		ID:        task.ID,
		From:      source,
		To:        agentdomain.SequentialWorkflowAgentName,
		Type:      agentdomain.TypeWorkflowSequentialStart,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"description":      task.Description,
			"org_id":           orgID,
			"trace_id":         task.TraceID,
			"task_id":          task.ID,
			"attempt":          task.Attempt,
			"parent_task_id":   task.ParentTaskID,
			"workflow_pattern": task.WorkflowPattern,
		},
	})

	return nil
}

func (e *ParallelExecutor) Execute(ctx context.Context, task *taskdomain.Task, input LaunchInput) error {
	if e == nil || e.bus == nil {
		return fmt.Errorf("workflow executor requires a message bus")
	}
	if task == nil {
		return fmt.Errorf("task is required")
	}

	source := input.Source
	if source == "" {
		source = "task_service"
	}

	orgID := input.OrgID
	if orgID == "" {
		orgID = "default"
	}

	payload := map[string]any{
		"description":      task.Description,
		"org_id":           orgID,
		"trace_id":         task.TraceID,
		"task_id":          task.ID,
		"attempt":          task.Attempt,
		"parent_task_id":   task.ParentTaskID,
		"workflow_pattern": task.WorkflowPattern,
	}
	if task.Input != nil {
		for key, value := range task.Input {
			payload[key] = value
		}
	}

	e.bus.Publish(ctx, agentdomain.Message{
		ID:        task.ID,
		From:      source,
		To:        agentdomain.ParallelWorkflowAgentName,
		Type:      agentdomain.TypeWorkflowParallelStart,
		Timestamp: time.Now(),
		Payload:   payload,
	})

	return nil
}

type CoordinatorExecutor struct {
	bus bus.Bus
}

func NewCoordinatorExecutor(b bus.Bus) *CoordinatorExecutor {
	return &CoordinatorExecutor{bus: b}
}

func (e *CoordinatorExecutor) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternCoordinator
}

func (e *CoordinatorExecutor) Execute(ctx context.Context, task *taskdomain.Task, input LaunchInput) error {
	if e == nil || e.bus == nil {
		return fmt.Errorf("workflow executor requires a message bus")
	}
	if task == nil {
		return fmt.Errorf("task is required")
	}

	source := input.Source
	if source == "" {
		source = "task_service"
	}

	orgID := input.OrgID
	if orgID == "" {
		orgID = "default"
	}

	payload := map[string]any{
		"description":      task.Description,
		"org_id":           orgID,
		"trace_id":         task.TraceID,
		"task_id":          task.ID,
		"attempt":          task.Attempt,
		"parent_task_id":   task.ParentTaskID,
		"workflow_pattern": task.WorkflowPattern,
	}
	if task.Input != nil {
		for key, value := range task.Input {
			payload[key] = value
		}
	}

	e.bus.Publish(ctx, agentdomain.Message{
		ID:        task.ID,
		From:      source,
		To:        agentdomain.CoordinatorWorkflowAgentName,
		Type:      agentdomain.TypeWorkflowCoordinatorStart,
		Timestamp: time.Now(),
		Payload:   payload,
	})

	return nil
}

type HierarchicalExecutor struct {
	bus bus.Bus
}

func NewHierarchicalExecutor(b bus.Bus) *HierarchicalExecutor {
	return &HierarchicalExecutor{bus: b}
}

func (e *HierarchicalExecutor) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternHierarchical
}

func (e *HierarchicalExecutor) Execute(ctx context.Context, task *taskdomain.Task, input LaunchInput) error {
	if e == nil || e.bus == nil {
		return fmt.Errorf("workflow executor requires a message bus")
	}
	if task == nil {
		return fmt.Errorf("task is required")
	}

	source := input.Source
	if source == "" {
		source = "task_service"
	}

	orgID := input.OrgID
	if orgID == "" {
		orgID = "default"
	}

	payload := map[string]any{
		"description":      task.Description,
		"org_id":           orgID,
		"trace_id":         task.TraceID,
		"task_id":          task.ID,
		"attempt":          task.Attempt,
		"parent_task_id":   task.ParentTaskID,
		"workflow_pattern": task.WorkflowPattern,
	}
	if task.Input != nil {
		for key, value := range task.Input {
			payload[key] = value
		}
	}

	e.bus.Publish(ctx, agentdomain.Message{
		ID:        task.ID,
		From:      source,
		To:        agentdomain.HierarchicalWorkflowAgentName,
		Type:      agentdomain.TypeWorkflowHierarchicalStart,
		Timestamp: time.Now(),
		Payload:   payload,
	})

	return nil
}

type LoopExecutor struct {
	bus       bus.Bus
	pattern   taskdomain.WorkflowPattern
	target    string
	startType string
}

func NewLoopExecutor(b bus.Bus) *LoopExecutor {
	return &LoopExecutor{
		bus:       b,
		pattern:   taskdomain.PatternLoop,
		target:    agentdomain.LoopWorkflowAgentName,
		startType: agentdomain.TypeWorkflowLoopStart,
	}
}

func NewReviewCritiqueExecutor(b bus.Bus) *LoopExecutor {
	return &LoopExecutor{
		bus:       b,
		pattern:   taskdomain.PatternReviewCritique,
		target:    agentdomain.ReviewCritiqueWorkflowAgentName,
		startType: agentdomain.TypeWorkflowReviewCritiqueStart,
	}
}

func NewIterativeRefinementExecutor(b bus.Bus) *LoopExecutor {
	return &LoopExecutor{
		bus:       b,
		pattern:   taskdomain.PatternIterativeRefinement,
		target:    agentdomain.IterativeRefinementWorkflowAgentName,
		startType: agentdomain.TypeWorkflowIterativeStart,
	}
}

func (e *LoopExecutor) Pattern() taskdomain.WorkflowPattern {
	if e == nil {
		return ""
	}
	return e.pattern
}

func (e *LoopExecutor) Execute(ctx context.Context, task *taskdomain.Task, input LaunchInput) error {
	if e == nil || e.bus == nil {
		return fmt.Errorf("workflow executor requires a message bus")
	}
	if task == nil {
		return fmt.Errorf("task is required")
	}

	source := input.Source
	if source == "" {
		source = "task_service"
	}

	orgID := input.OrgID
	if orgID == "" {
		orgID = "default"
	}

	e.bus.Publish(ctx, agentdomain.Message{
		ID:        task.ID,
		From:      source,
		To:        e.target,
		Type:      e.startType,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"description":      task.Description,
			"org_id":           orgID,
			"trace_id":         task.TraceID,
			"task_id":          task.ID,
			"attempt":          task.Attempt,
			"parent_task_id":   task.ParentTaskID,
			"workflow_pattern": task.WorkflowPattern,
			"max_iterations":   reviewCritiqueMaxIterations(task.Input),
		},
	})

	return nil
}

func reviewCritiqueMaxIterations(input map[string]any) int {
	if input == nil {
		return 3
	}
	if value, ok := input[taskdomain.MaxReviewIterationsInputKey]; ok {
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
	}
	return 3
}

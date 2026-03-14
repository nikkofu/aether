package task

import (
	"context"
	"strings"
	"time"
)

type Status string
type WorkflowPattern string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusReviewing   Status = "reviewing"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
	StatusInterrupted Status = "interrupted"
)

const (
	PatternSequential          WorkflowPattern = "sequential"
	PatternParallel            WorkflowPattern = "parallel"
	PatternLoop                WorkflowPattern = "loop"
	PatternReviewCritique      WorkflowPattern = "review_critique"
	PatternIterativeRefinement WorkflowPattern = "iterative_refinement"
	PatternCoordinator         WorkflowPattern = "coordinator"
	PatternHierarchical        WorkflowPattern = "hierarchical"
)

func NormalizeWorkflowPattern(pattern WorkflowPattern) WorkflowPattern {
	normalized := WorkflowPattern(strings.ToLower(strings.TrimSpace(string(pattern))))

	switch normalized {
	case "", PatternSequential:
		return PatternSequential
	case "prompt_chaining", "orchestrator_workers":
		return PatternSequential
	case "parallelization":
		return PatternParallel
	case "routing":
		return PatternCoordinator
	case "evaluator_optimizer":
		return PatternReviewCritique
	default:
		return normalized
	}
}

func IsValidWorkflowPattern(pattern WorkflowPattern) bool {
	switch NormalizeWorkflowPattern(pattern) {
	case PatternSequential,
		PatternParallel,
		PatternLoop,
		PatternReviewCritique,
		PatternIterativeRefinement,
		PatternCoordinator,
		PatternHierarchical:
		return true
	default:
		return false
	}
}

type Task struct {
	ID              string          `json:"id"`
	ParentTaskID    string          `json:"parent_task_id,omitempty"`
	Attempt         int             `json:"attempt"`
	Source          string          `json:"source"`
	Mode            string          `json:"mode"`
	WorkflowPattern WorkflowPattern `json:"workflow_pattern"`
	Description     string          `json:"description"`
	Input           map[string]any  `json:"input,omitempty"`
	Status          Status          `json:"status"`
	TraceID         string          `json:"trace_id,omitempty"`
	CurrentStage    string          `json:"current_stage,omitempty"`
	FinalOutput     string          `json:"final_output,omitempty"`
	ErrorSummary    string          `json:"error_summary,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Event struct {
	ID        int64          `json:"id"`
	TaskID    string         `json:"task_id"`
	MessageID string         `json:"message_id,omitempty"`
	From      string         `json:"from,omitempty"`
	To        string         `json:"to,omitempty"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Store interface {
	Create(ctx context.Context, t *Task) error
	Get(ctx context.Context, id string) (*Task, error)
	List(ctx context.Context, limit int) ([]*Task, error)
	ListByStatus(ctx context.Context, statuses []Status, limit int) ([]*Task, error)
	Update(ctx context.Context, t *Task) error
	AppendEvent(ctx context.Context, e *Event) error
	ListEvents(ctx context.Context, taskID string, limit int) ([]*Event, error)
}

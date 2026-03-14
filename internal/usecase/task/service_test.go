package task

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/pkg/logging"
	_ "modernc.org/sqlite"
)

type noopLogger struct{}

func (n *noopLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (n *noopLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (n *noopLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (n *noopLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (n *noopLogger) Sync() error                                                    { return nil }

type stubTerminalObserver struct {
	calls  int
	task   *taskdomain.Task
	events []*taskdomain.Event
}

func (s *stubTerminalObserver) ObserveTerminalTask(ctx context.Context, task *taskdomain.Task, events []*taskdomain.Event) error {
	s.calls++
	s.task = task
	s.events = events
	return nil
}

func setupTaskServiceTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxOpenConns(1)

	return db, func() { _ = db.Close() }
}

func TestServiceRecoverInterrupted(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	service := NewService(store, nil, &noopLogger{})

	runningTask := &taskdomain.Task{
		ID:           "task-running",
		Attempt:      1,
		Source:       "api",
		Mode:         "agent",
		Description:  "Recover me after restart",
		Status:       taskdomain.StatusRunning,
		CurrentStage: "coder",
	}
	if err := store.Create(context.Background(), runningTask); err != nil {
		t.Fatalf("failed to create running task: %v", err)
	}

	completedTask := &taskdomain.Task{
		ID:           "task-completed",
		Attempt:      1,
		Source:       "api",
		Mode:         "agent",
		Description:  "Leave me alone",
		Status:       taskdomain.StatusCompleted,
		CurrentStage: "completed",
	}
	if err := store.Create(context.Background(), completedTask); err != nil {
		t.Fatalf("failed to create completed task: %v", err)
	}

	recovered, err := service.RecoverInterrupted(context.Background())
	if err != nil {
		t.Fatalf("recover interrupted failed: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("expected 1 recovered task, got %d", recovered)
	}

	taskAfterRecovery, err := service.Get(context.Background(), runningTask.ID)
	if err != nil {
		t.Fatalf("failed to reload recovered task: %v", err)
	}
	if taskAfterRecovery.Status != taskdomain.StatusInterrupted {
		t.Fatalf("expected interrupted status, got %s", taskAfterRecovery.Status)
	}
	if taskAfterRecovery.CurrentStage != "interrupted" {
		t.Fatalf("expected interrupted stage, got %s", taskAfterRecovery.CurrentStage)
	}
	if taskAfterRecovery.ErrorSummary == "" {
		t.Fatal("expected recovery error summary to be populated")
	}

	events, err := service.ListEvents(context.Background(), runningTask.ID, 10)
	if err != nil {
		t.Fatalf("failed to load recovered task events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 recovery event, got %d", len(events))
	}
	if events[0].Type != "task.interrupted" {
		t.Fatalf("expected task.interrupted event, got %s", events[0].Type)
	}

	completedAfterRecovery, err := service.Get(context.Background(), completedTask.ID)
	if err != nil {
		t.Fatalf("failed to reload completed task: %v", err)
	}
	if completedAfterRecovery.Status != taskdomain.StatusCompleted {
		t.Fatalf("expected completed task to remain completed, got %s", completedAfterRecovery.Status)
	}
}

func TestServiceRetryInterruptedTask(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	original := &taskdomain.Task{
		ID:           "task-interrupted",
		Attempt:      1,
		Source:       "api",
		Mode:         "agent",
		Description:  "Retry after interruption",
		Status:       taskdomain.StatusInterrupted,
		CurrentStage: "interrupted",
		ErrorSummary: "Interrupted by daemon restart",
	}
	if err := store.Create(context.Background(), original); err != nil {
		t.Fatalf("failed to create interrupted task: %v", err)
	}

	retriedTask, err := service.Retry(context.Background(), original.ID)
	if err != nil {
		t.Fatalf("retry interrupted task failed: %v", err)
	}

	if retriedTask.Attempt != 2 {
		t.Fatalf("expected retried task attempt 2, got %d", retriedTask.Attempt)
	}
	if retriedTask.ParentTaskID != original.ID {
		t.Fatalf("expected parent task ID %s, got %s", original.ID, retriedTask.ParentTaskID)
	}
	if retriedTask.Status != taskdomain.StatusRunning {
		t.Fatalf("expected retried task to be running, got %s", retriedTask.Status)
	}
	if retriedTask.CurrentStage != "workflow.sequential" {
		t.Fatalf("expected sequential retry to start at workflow agent, got %s", retriedTask.CurrentStage)
	}
	if retriedTask.WorkflowPattern != taskdomain.PatternSequential {
		t.Fatalf("expected default workflow pattern, got %s", retriedTask.WorkflowPattern)
	}
	if retryOf, ok := retriedTask.Input["retry_of"].(string); !ok || retryOf != original.ID {
		t.Fatalf("expected retry_of=%s, got %#v", original.ID, retriedTask.Input["retry_of"])
	}
}

func TestServiceSubmitRejectsNonTaxonomyWorkflowPattern(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	_, err = service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.WorkflowPattern("swarm"),
		Description:     "Route this through a swarm",
	})
	if err == nil {
		t.Fatal("expected invalid workflow pattern error")
	}
	if !errors.Is(err, ErrWorkflowPatternInvalid) {
		t.Fatalf("expected ErrWorkflowPatternInvalid, got %v", err)
	}
}

func TestServiceSubmitRejectsBlankDescription(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	_, err = service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternSequential,
		Description:     "   \n\t  ",
	})
	if err == nil {
		t.Fatal("expected blank description to be rejected")
	}
	if err.Error() != "task description is required" {
		t.Fatalf("expected task description validation error, got %v", err)
	}
}

func TestServiceSubmitTrimsDescriptionAndWorkflowPattern(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          " api ",
		Mode:            " agent ",
		WorkflowPattern: taskdomain.WorkflowPattern(" Parallel "),
		Description:     "  Ship the parallel branch plan  ",
	})
	if err != nil {
		t.Fatalf("expected trimmed submit to succeed: %v", err)
	}
	if task.Description != "Ship the parallel branch plan" {
		t.Fatalf("expected trimmed description, got %q", task.Description)
	}
	if task.WorkflowPattern != taskdomain.PatternParallel {
		t.Fatalf("expected normalized workflow pattern, got %s", task.WorkflowPattern)
	}
	if task.Source != "api" {
		t.Fatalf("expected trimmed source, got %q", task.Source)
	}
	if task.Mode != "agent" {
		t.Fatalf("expected trimmed mode, got %q", task.Mode)
	}
}

func TestServiceSubmitHierarchicalTask(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternHierarchical,
		Description:     "Execute this via the hierarchy",
		Input: map[string]any{
			"goal_id": "goal-h1",
		},
	})
	if err != nil {
		t.Fatalf("expected hierarchical submit to succeed: %v", err)
	}
	if task.WorkflowPattern != taskdomain.PatternHierarchical {
		t.Fatalf("expected hierarchical pattern, got %s", task.WorkflowPattern)
	}
	if task.Status != taskdomain.StatusRunning {
		t.Fatalf("expected running task, got %s", task.Status)
	}
	if task.CurrentStage != "workflow.hierarchical" {
		t.Fatalf("expected hierarchical task to start at workflow agent, got %s", task.CurrentStage)
	}
}

func TestServiceSubmitLoopTask(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternLoop,
		Description:     "Run this through the generic loop workflow",
		Input: map[string]any{
			"max_review_iterations": 3,
		},
	})
	if err != nil {
		t.Fatalf("expected loop submit to succeed: %v", err)
	}
	if task.WorkflowPattern != taskdomain.PatternLoop {
		t.Fatalf("expected loop pattern, got %s", task.WorkflowPattern)
	}
	if task.Status != taskdomain.StatusRunning {
		t.Fatalf("expected running task, got %s", task.Status)
	}
	if task.CurrentStage != "workflow.loop" {
		t.Fatalf("expected loop task to start at workflow agent, got %s", task.CurrentStage)
	}
}

func TestServiceSubmitParallelTask(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternParallel,
		Description:     "Run this through the explicit parallel workflow",
		Input: map[string]any{
			"parallel_branches": []any{"analysis branch", "implementation branch"},
		},
	})
	if err != nil {
		t.Fatalf("expected parallel submit to succeed: %v", err)
	}
	if task.WorkflowPattern != taskdomain.PatternParallel {
		t.Fatalf("expected parallel pattern, got %s", task.WorkflowPattern)
	}
	if task.Status != taskdomain.StatusRunning {
		t.Fatalf("expected running task, got %s", task.Status)
	}
	if task.CurrentStage != "workflow.parallel" {
		t.Fatalf("expected parallel task to start at workflow agent, got %s", task.CurrentStage)
	}
	expectedBranches := []map[string]any{
		{"task": "analysis branch"},
		{"task": "implementation branch"},
	}
	if actual, ok := task.Input[taskdomain.ParallelBranchesInputKey].([]map[string]any); !ok || !reflect.DeepEqual(actual, expectedBranches) {
		t.Fatalf("expected canonical parallel branches %#v, got %#v", expectedBranches, task.Input[taskdomain.ParallelBranchesInputKey])
	}
}

func TestServiceSubmitParallelTaskNormalizesLegacyBranchKey(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternParallel,
		Description:     "Normalize legacy branch key",
		Input: map[string]any{
			taskdomain.LegacyParallelTasksInputKey: []any{
				"Plan::Analyze architecture",
				map[string]any{"name": "Build", "task": "Implement branch fan-out"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected parallel submit with legacy branch key to succeed: %v", err)
	}

	expectedBranches := []map[string]any{
		{"name": "Plan", "task": "Analyze architecture"},
		{"name": "Build", "task": "Implement branch fan-out"},
	}
	if actual, ok := task.Input[taskdomain.ParallelBranchesInputKey].([]map[string]any); !ok || !reflect.DeepEqual(actual, expectedBranches) {
		t.Fatalf("expected canonical parallel branches %#v, got %#v", expectedBranches, task.Input[taskdomain.ParallelBranchesInputKey])
	}
	if _, exists := task.Input[taskdomain.LegacyParallelTasksInputKey]; exists {
		t.Fatalf("expected legacy branch key to be removed, got %#v", task.Input)
	}
}

func TestServiceSubmitCoordinatorTask(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternCoordinator,
		Description:     "Coordinate this implementation across workers",
		Input: map[string]any{
			"goal_id":      "goal-1",
			"milestone_id": "ms-1",
		},
	})
	if err != nil {
		t.Fatalf("expected coordinator submit to succeed: %v", err)
	}
	if task.WorkflowPattern != taskdomain.PatternCoordinator {
		t.Fatalf("expected coordinator pattern, got %s", task.WorkflowPattern)
	}
	if task.Status != taskdomain.StatusRunning {
		t.Fatalf("expected running task, got %s", task.Status)
	}
	if task.CurrentStage != "workflow.coordinator" {
		t.Fatalf("expected coordinator task to start at workflow agent, got %s", task.CurrentStage)
	}
}

func TestServiceSubmitReviewCritiqueTask(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternReviewCritique,
		Description:     "Iteratively improve this implementation",
		Input: map[string]any{
			"max_review_iterations": 4,
		},
	})
	if err != nil {
		t.Fatalf("expected review_critique submit to succeed: %v", err)
	}
	if task.WorkflowPattern != taskdomain.PatternReviewCritique {
		t.Fatalf("expected review_critique pattern, got %s", task.WorkflowPattern)
	}
	if task.Status != taskdomain.StatusRunning {
		t.Fatalf("expected running task, got %s", task.Status)
	}
	if task.CurrentStage != "workflow.review_critique" {
		t.Fatalf("expected review_critique task to start at workflow agent, got %s", task.CurrentStage)
	}
}

func TestServiceSubmitIterativeRefinementTask(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	messageBus := bus.NewMemoryBus(16)
	service := NewService(store, messageBus, &noopLogger{})

	task, err := service.Submit(context.Background(), SubmitInput{
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternIterativeRefinement,
		Description:     "Official iterative refinement workflow",
		Input: map[string]any{
			"max_review_iterations": 5,
		},
	})
	if err != nil {
		t.Fatalf("expected iterative_refinement submit to succeed: %v", err)
	}
	if task.WorkflowPattern != taskdomain.PatternIterativeRefinement {
		t.Fatalf("expected iterative_refinement pattern, got %s", task.WorkflowPattern)
	}
	if task.Status != taskdomain.StatusRunning {
		t.Fatalf("expected running task, got %s", task.Status)
	}
	if task.CurrentStage != "workflow.iterative_refinement" {
		t.Fatalf("expected iterative_refinement task to start at workflow agent, got %s", task.CurrentStage)
	}
}

func TestServiceObservesTerminalTaskOnceOnCompletion(t *testing.T) {
	db, cleanup := setupTaskServiceTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	service := NewService(store, nil, &noopLogger{})
	observer := &stubTerminalObserver{}
	service.SetTerminalTaskObserver(observer)

	task := &taskdomain.Task{
		ID:              "task-terminal-1",
		Attempt:         1,
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: taskdomain.PatternReviewCritique,
		Description:     "Observe terminal completion",
		Status:          taskdomain.StatusRunning,
		CurrentStage:    "workflow.review_critique",
	}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	service.handleMessage(agentdomain.Message{
		ID:   task.ID,
		From: "workflow.review_critique",
		To:   "cli",
		Type: "final_report",
		Payload: map[string]any{
			"task_id": task.ID,
			"result":  "done",
		},
	})

	if observer.calls != 1 {
		t.Fatalf("expected terminal observer to be called once, got %d", observer.calls)
	}
	if observer.task == nil || observer.task.Status != taskdomain.StatusCompleted {
		t.Fatalf("expected observed task to be completed, got %#v", observer.task)
	}
	if len(observer.events) != 1 || observer.events[0].Type != "final_report" {
		t.Fatalf("expected observer to receive the terminal final_report event, got %#v", observer.events)
	}
}

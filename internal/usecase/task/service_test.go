package task

import (
	"context"
	"database/sql"
	"testing"

	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	"github.com/nikkofu/aether/pkg/logging"
	_ "modernc.org/sqlite"
)

type noopLogger struct{}

func (n *noopLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (n *noopLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (n *noopLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (n *noopLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (n *noopLogger) Sync() error                                                    { return nil }

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

	service := NewService(store, nil, &noopLogger{})

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
	if retryOf, ok := retriedTask.Input["retry_of"].(string); !ok || retryOf != original.ID {
		t.Fatalf("expected retry_of=%s, got %#v", original.ID, retriedTask.Input["retry_of"])
	}
}

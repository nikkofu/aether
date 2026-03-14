package task

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTaskSQLiteTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxOpenConns(1)

	return db, func() { _ = db.Close() }
}

func TestSQLiteStoreCanonicalizesParallelTaskInput(t *testing.T) {
	db, cleanup := setupTaskSQLiteTestDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	task := &Task{
		ID:              "task-parallel-store",
		Attempt:         1,
		Source:          "api",
		Mode:            "agent",
		WorkflowPattern: PatternParallel,
		Description:     "Persist canonical parallel branches",
		Input: map[string]any{
			LegacyParallelTasksInputKey: []any{
				"Plan::Analyze architecture",
				map[string]any{"name": "Build", "task": "Implement branch fan-out"},
			},
		},
		Status: StatusQueued,
	}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	expectedBranches := []map[string]any{
		{"name": "Plan", "task": "Analyze architecture"},
		{"name": "Build", "task": "Implement branch fan-out"},
	}
	if actual, ok := task.Input[ParallelBranchesInputKey].([]map[string]any); !ok || !reflect.DeepEqual(actual, expectedBranches) {
		t.Fatalf("expected canonical in-memory branches %#v, got %#v", expectedBranches, task.Input[ParallelBranchesInputKey])
	}

	storedTask, err := store.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("failed to reload task: %v", err)
	}
	if actual, ok := storedTask.Input[ParallelBranchesInputKey].([]map[string]any); !ok || !reflect.DeepEqual(actual, expectedBranches) {
		t.Fatalf("expected canonical stored branches %#v, got %#v", expectedBranches, storedTask.Input[ParallelBranchesInputKey])
	}
	if _, exists := storedTask.Input[LegacyParallelTasksInputKey]; exists {
		t.Fatalf("expected legacy key to be removed after reload, got %#v", storedTask.Input)
	}

	listedTasks, err := store.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(listedTasks) != 1 {
		t.Fatalf("expected 1 listed task, got %d", len(listedTasks))
	}
	if actual, ok := listedTasks[0].Input[ParallelBranchesInputKey].([]map[string]any); !ok || !reflect.DeepEqual(actual, expectedBranches) {
		t.Fatalf("expected canonical listed branches %#v, got %#v", expectedBranches, listedTasks[0].Input[ParallelBranchesInputKey])
	}
}

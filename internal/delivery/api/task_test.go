package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/pkg/logging"
	_ "modernc.org/sqlite"
)

type testLogger struct{}

func (t *testLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (t *testLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (t *testLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (t *testLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (t *testLogger) Sync() error                                                    { return nil }

func setupTaskTestDB(t *testing.T) (*sql.DB, func()) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db, func() { _ = db.Close() }
}

func TestTaskAPI(t *testing.T) {
	db, cleanup := setupTaskTestDB(t)
	defer cleanup()

	store, err := taskdomain.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	b := bus.NewMemoryBus(100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Start(ctx)

	service := taskusecase.NewService(store, b, &testLogger{})
	service.StartObservers(context.Background())

	handler := NewTaskHandler(service, &testLogger{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	var taskID string
	var retriedTaskID string

	t.Run("create task", func(t *testing.T) {
		body := []byte(`{"description":"Design a task control plane"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}

		var taskResp taskdomain.Task
		if err := json.NewDecoder(w.Body).Decode(&taskResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if taskResp.Description != "Design a task control plane" {
			t.Fatalf("unexpected task description: %s", taskResp.Description)
		}
		if taskResp.Status != taskdomain.StatusRunning {
			t.Fatalf("expected running status, got %s", taskResp.Status)
		}
		if taskResp.Attempt != 1 {
			t.Fatalf("expected attempt 1, got %d", taskResp.Attempt)
		}

		taskID = taskResp.ID
	})

	t.Run("cancel task", func(t *testing.T) {
		body := []byte(`{"reason":"Stop the current attempt"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID+"/cancel", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var taskResp taskdomain.Task
		if err := json.NewDecoder(w.Body).Decode(&taskResp); err != nil {
			t.Fatalf("failed to decode cancelled task: %v", err)
		}
		if taskResp.Status != taskdomain.StatusCancelled {
			t.Fatalf("expected cancelled status, got %s", taskResp.Status)
		}
	})

	t.Run("retry task", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID+"/retry", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("expected status 202, got %d", w.Code)
		}

		var taskResp taskdomain.Task
		if err := json.NewDecoder(w.Body).Decode(&taskResp); err != nil {
			t.Fatalf("failed to decode retried task: %v", err)
		}
		if taskResp.ID == taskID {
			t.Fatal("expected retried task to have a new ID")
		}
		if taskResp.Status != taskdomain.StatusRunning {
			t.Fatalf("expected retried task to be running, got %s", taskResp.Status)
		}
		if taskResp.Attempt != 2 {
			t.Fatalf("expected retried task attempt 2, got %d", taskResp.Attempt)
		}
		if taskResp.ParentTaskID != taskID {
			t.Fatalf("expected parent task ID %s, got %s", taskID, taskResp.ParentTaskID)
		}

		retriedTaskID = taskResp.ID
	})

	t.Run("list tasks", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var tasks []*taskdomain.Task
		if err := json.NewDecoder(w.Body).Decode(&tasks); err != nil {
			t.Fatalf("failed to decode task list: %v", err)
		}
		if len(tasks) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(tasks))
		}
	})

	t.Run("get task", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var taskResp taskdomain.Task
		if err := json.NewDecoder(w.Body).Decode(&taskResp); err != nil {
			t.Fatalf("failed to decode task: %v", err)
		}
		if taskResp.ID != taskID {
			t.Fatalf("expected task ID %s, got %s", taskID, taskResp.ID)
		}
		if taskResp.Status != taskdomain.StatusCancelled {
			t.Fatalf("expected cancelled status, got %s", taskResp.Status)
		}
	})

	t.Run("list task events", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID+"/events", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var events []*taskdomain.Event
		if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
			t.Fatalf("failed to decode events: %v", err)
		}
		if len(events) == 0 {
			t.Fatal("expected at least one task event")
		}
	})

	t.Run("get retried task", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+retriedTaskID, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var taskResp taskdomain.Task
		if err := json.NewDecoder(w.Body).Decode(&taskResp); err != nil {
			t.Fatalf("failed to decode retried task: %v", err)
		}
		if retryOf, ok := taskResp.Input["retry_of"].(string); !ok || retryOf != taskID {
			t.Fatalf("expected retry_of=%s, got %#v", taskID, taskResp.Input["retry_of"])
		}
	})
}

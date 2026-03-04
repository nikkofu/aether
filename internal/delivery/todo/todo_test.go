package todo

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nikkofu/aether/pkg/logging"
	_ "modernc.org/sqlite"
)

type dummyLogger struct{}

func (d *dummyLogger) Debug(ctx context.Context, msg string, fields ...logging.Field) {}
func (d *dummyLogger) Info(ctx context.Context, msg string, fields ...logging.Field)  {}
func (d *dummyLogger) Warn(ctx context.Context, msg string, fields ...logging.Field)  {}
func (d *dummyLogger) Error(ctx context.Context, msg string, fields ...logging.Field) {}
func (d *dummyLogger) Sync() error                                                 { return nil }

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	return db, func() {
		db.Close()
	}
}

func TestTodoAPI(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	handler := NewHandler(store, &dummyLogger{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	var todoID string

	// 1. 测试创建任务 (POST)
	t.Run("CreateTodo", func(t *testing.T) {
		body := []byte(`{"title": "Test Todo", "description": "Test Description"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/todos", bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d", w.Code)
		}

		var created Todo
		if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if created.Title != "Test Todo" {
			t.Errorf("expected title 'Test Todo', got %s", created.Title)
		}
		todoID = created.ID
	})

	// 2. 测试获取列表 (GET)
	t.Run("ListTodos", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/todos", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var todos []*Todo
		if err := json.NewDecoder(w.Body).Decode(&todos); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(todos) != 1 {
			t.Errorf("expected 1 todo, got %d", len(todos))
		}
	})

	// 3. 测试获取详情 (GET)
	t.Run("GetTodo", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/todos/"+todoID, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var todo Todo
		if err := json.NewDecoder(w.Body).Decode(&todo); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if todo.ID != todoID {
			t.Errorf("expected ID %s, got %s", todoID, todo.ID)
		}
	})

	// 4. 测试更新任务 (PUT)
	t.Run("UpdateTodo", func(t *testing.T) {
		body := []byte(`{"title": "Updated Todo", "description": "Updated Description", "completed": true}`)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/todos/"+todoID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var updated Todo
		if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if updated.Title != "Updated Todo" || !updated.Completed {
			t.Errorf("update failed: %+v", updated)
		}
	})

	// 5. 测试部分更新 (PATCH)
	t.Run("PatchTodo", func(t *testing.T) {
		body := []byte(`{"completed": false}`)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/todos/"+todoID, bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var patched Todo
		if err := json.NewDecoder(w.Body).Decode(&patched); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if patched.Completed {
			t.Errorf("patch failed: %+v", patched)
		}
	})

	// 6. 测试删除任务 (DELETE)
	t.Run("DeleteTodo", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/todos/"+todoID, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", w.Code)
		}

		// 验证已删除
		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/todos/"+todoID, nil)
		wGet := httptest.NewRecorder()
		mux.ServeHTTP(wGet, reqGet)
		if wGet.Code != http.StatusNotFound {
			t.Errorf("expected status 404 after deletion, got %d", wGet.Code)
		}
	})
}

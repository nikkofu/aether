package todo

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/pkg/logging"
)

// Handler 包装了待办事项的 HTTP 处理逻辑。
type Handler struct {
	store  Store
	logger logging.Logger
}

// NewHandler 创建一个新的待办事项处理器。
func NewHandler(store Store, logger logging.Logger) *Handler {
	return &Handler{
		store:  store,
		logger: logger,
	}
}

// RegisterRoutes 在提供的 ServeMux 上注册路由。
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/todos", h.handleTodos)
	mux.HandleFunc("/api/v1/todos/", h.handleTodo)
}

func (h *Handler) sendError(w http.ResponseWriter, r *http.Request, status int, code, message string, err error) {
	requestID := uuid.New().String()
	
	if err != nil {
		h.logger.Error(r.Context(), "API Error", 
			logging.String("code", code),
			logging.String("message", message),
			logging.String("request_id", requestID),
			logging.Err(err),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":      code,
		"message":    message,
		"request_id": requestID,
	})
}

func (h *Handler) handleTodos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTodos(w, r)
	case http.MethodPost:
		h.createTodo(w, r)
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusOK)
	default:
		h.sendError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
	}
}

func (h *Handler) handleTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/todos/")
	if id == "" {
		h.sendError(w, r, http.StatusBadRequest, "id_required", "ID is required", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getTodo(w, r, id)
	case http.MethodPut:
		h.updateTodo(w, r, id)
	case http.MethodPatch:
		h.patchTodo(w, r, id)
	case http.MethodDelete:
		h.deleteTodo(w, r, id)
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, PATCH, DELETE, OPTIONS")
		w.WriteHeader(http.StatusOK)
	default:
		h.sendError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
	}
}

func (h *Handler) listTodos(w http.ResponseWriter, r *http.Request) {
	todos, err := h.store.List()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "internal_error", "Failed to list todos", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(todos)
}

func (h *Handler) createTodo(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body", err)
		return
	}

	if input.Title == "" {
		h.sendError(w, r, http.StatusBadRequest, "validation_failed", "标题不能为空", nil)
		return
	}

	todo := &Todo{
		ID:          uuid.New().String(),
		Title:       input.Title,
		Description: input.Description,
		Completed:   false,
	}

	if err := h.store.Create(todo); err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "internal_error", "Failed to create todo", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(todo)
}

func (h *Handler) getTodo(w http.ResponseWriter, r *http.Request, id string) {
	todo, err := h.store.Get(id)
	if err != nil {
		h.sendError(w, r, http.StatusNotFound, "not_found", "找不到指定 ID 的任务", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(todo)
}

func (h *Handler) updateTodo(w http.ResponseWriter, r *http.Request, id string) {
	todo, err := h.store.Get(id)
	if err != nil {
		h.sendError(w, r, http.StatusNotFound, "not_found", "找不到指定 ID 的任务", err)
		return
	}

	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Completed   bool   `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body", err)
		return
	}

	if input.Title == "" {
		h.sendError(w, r, http.StatusBadRequest, "validation_failed", "标题不能为空", nil)
		return
	}

	todo.Title = input.Title
	todo.Description = input.Description
	todo.Completed = input.Completed

	if err := h.store.Update(todo); err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "internal_error", "Failed to update todo", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(todo)
}

func (h *Handler) patchTodo(w http.ResponseWriter, r *http.Request, id string) {
	todo, err := h.store.Get(id)
	if err != nil {
		h.sendError(w, r, http.StatusNotFound, "not_found", "找不到指定 ID 的任务", err)
		return
	}

	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body", err)
		return
	}

	if title, ok := input["title"].(string); ok {
		if title == "" {
			h.sendError(w, r, http.StatusBadRequest, "validation_failed", "标题不能为空", nil)
			return
		}
		todo.Title = title
	}
	if desc, ok := input["description"].(string); ok {
		todo.Description = desc
	}
	if completed, ok := input["completed"].(bool); ok {
		todo.Completed = completed
	}

	if err := h.store.Update(todo); err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "internal_error", "Failed to patch todo", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(todo)
}

func (h *Handler) deleteTodo(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.store.Delete(id); err != nil {
		h.sendError(w, r, http.StatusNotFound, "not_found", "找不到指定 ID 的任务", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


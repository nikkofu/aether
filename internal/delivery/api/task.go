package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
	"github.com/nikkofu/aether/pkg/logging"
)

type TaskHandler struct {
	service *taskusecase.Service
	logger  logging.Logger
}

func NewTaskHandler(service *taskusecase.Service, logger logging.Logger) *TaskHandler {
	return &TaskHandler{
		service: service,
		logger:  logger,
	}
}

func (h *TaskHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/tasks", h.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", h.handleTask)
}

func (h *TaskHandler) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTasks(w, r)
	case http.MethodPost:
		h.createTask(w, r)
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusOK)
	default:
		h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
	}
}

func (h *TaskHandler) handleTask(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if path == "" {
		h.writeError(w, r, http.StatusBadRequest, "task_id_required", "Task ID is required", nil)
		return
	}

	if strings.HasSuffix(path, "/events/stream") {
		id := strings.TrimSuffix(path, "/events/stream")
		id = strings.TrimSuffix(id, "/")
		if r.Method != http.MethodGet {
			h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
			return
		}
		h.streamTaskEvents(w, r, id)
		return
	}

	if strings.HasSuffix(path, "/events") {
		id := strings.TrimSuffix(path, "/events")
		id = strings.TrimSuffix(id, "/")
		if r.Method != http.MethodGet {
			h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
			return
		}
		h.listTaskEvents(w, r, id)
		return
	}

	if strings.HasSuffix(path, "/retry") {
		id := strings.TrimSuffix(path, "/retry")
		id = strings.TrimSuffix(id, "/")
		if r.Method != http.MethodPost {
			h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
			return
		}
		h.retryTask(w, r, id)
		return
	}

	if strings.HasSuffix(path, "/cancel") {
		id := strings.TrimSuffix(path, "/cancel")
		id = strings.TrimSuffix(id, "/")
		if r.Method != http.MethodPost {
			h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
			return
		}
		h.cancelTask(w, r, id)
		return
	}

	if r.Method != http.MethodGet {
		h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
		return
	}
	h.getTask(w, r, path)
}

func (h *TaskHandler) createTask(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Source          string                     `json:"source"`
		Mode            string                     `json:"mode"`
		WorkflowPattern taskdomain.WorkflowPattern `json:"workflow_pattern"`
		Description     string                     `json:"description"`
		Input           map[string]any             `json:"input"`
		TraceID         string                     `json:"trace_id"`
		OrgID           string                     `json:"org_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body", err)
		return
	}

	task, err := h.service.Submit(r.Context(), taskusecase.SubmitInput{
		Source:          input.Source,
		Mode:            input.Mode,
		WorkflowPattern: input.WorkflowPattern,
		Description:     input.Description,
		Input:           input.Input,
		TraceID:         input.TraceID,
		OrgID:           input.OrgID,
	})
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "task_submit_failed", err.Error(), err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(task)
}

func (h *TaskHandler) listTasks(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	tasks, err := h.service.List(r.Context(), limit)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "task_list_failed", "Failed to list tasks", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks)
}

func (h *TaskHandler) getTask(w http.ResponseWriter, r *http.Request, id string) {
	task, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeTaskError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(task)
}

func (h *TaskHandler) listTaskEvents(w http.ResponseWriter, r *http.Request, id string) {
	events, err := h.service.ListEvents(r.Context(), id, 200)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "task_events_failed", "Failed to list task events", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

func (h *TaskHandler) retryTask(w http.ResponseWriter, r *http.Request, id string) {
	task, err := h.service.Retry(r.Context(), id)
	if err != nil {
		h.writeTaskError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(task)
}

func (h *TaskHandler) cancelTask(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			h.writeError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body", err)
			return
		}
	}

	task, err := h.service.Cancel(r.Context(), id, input.Reason)
	if err != nil {
		h.writeTaskError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(task)
}

func (h *TaskHandler) streamTaskEvents(w http.ResponseWriter, r *http.Request, id string) {
	task, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeTaskError(w, r, err)
		return
	}

	events, err := h.service.ListEvents(r.Context(), id, 200)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "task_events_failed", "Failed to list task events", err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, r, http.StatusInternalServerError, "stream_unsupported", "Streaming unsupported", nil)
		return
	}

	updates, cancel := h.service.Subscribe(id, 64)
	defer cancel()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if err := h.writeSSE(w, "snapshot", map[string]any{
		"task":   task,
		"events": events,
	}); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			if err := h.writeSSE(w, "update", update); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *TaskHandler) writeSSE(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

func (h *TaskHandler) writeTaskError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, taskusecase.ErrTaskNotFound):
		h.writeError(w, r, http.StatusNotFound, "task_not_found", "Task not found", err)
	case errors.Is(err, taskusecase.ErrTaskNotCancelable):
		h.writeError(w, r, http.StatusConflict, "task_not_cancelable", err.Error(), err)
	case errors.Is(err, taskusecase.ErrTaskNotRetryable):
		h.writeError(w, r, http.StatusConflict, "task_not_retryable", err.Error(), err)
	default:
		h.writeError(w, r, http.StatusInternalServerError, "task_operation_failed", "Task operation failed", err)
	}
}

func (h *TaskHandler) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, err error) {
	if err != nil && h.logger != nil {
		h.logger.Error(r.Context(), "task API error",
			logging.String("code", code),
			logging.String("message", message),
			logging.Err(err),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}

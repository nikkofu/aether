package api

import (
	"encoding/json"
	"net/http"
	"time"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/pkg/logging"
)

type SystemHandler struct {
	taskService  *taskusecase.Service
	bus          bus.Bus
	agentManager agentdomain.AgentManager
	logger       logging.Logger
}

func NewSystemHandler(taskService *taskusecase.Service, b bus.Bus, agentManager agentdomain.AgentManager, logger logging.Logger) *SystemHandler {
	return &SystemHandler{
		taskService:  taskService,
		bus:          b,
		agentManager: agentManager,
		logger:       logger,
	}
}

func (h *SystemHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", h.handleHealth)
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
}

func (h *SystemHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
		return
	}

	status := "ok"
	components := map[string]string{
		"task_service":  componentStatus(h.taskService != nil),
		"bus":           componentStatus(h.bus != nil),
		"agent_manager": componentStatus(h.agentManager != nil),
	}

	if h.taskService == nil || h.bus == nil || h.agentManager == nil {
		status = "degraded"
	}

	var stats agentdomain.ManagerStats
	if h.agentManager != nil {
		stats = h.agentManager.GetStats()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      status,
		"timestamp":   time.Now().UTC(),
		"components":  components,
		"agent_stats": stats,
	})
}

func (h *SystemHandler) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
		return
	}

	agents := make([]map[string]any, 0)
	stats := agentdomain.ManagerStats{}
	if h.agentManager != nil {
		for _, current := range h.agentManager.List() {
			agents = append(agents, map[string]any{
				"name":     current.Name(),
				"role":     current.Role(),
				"status":   current.Status(),
				"metadata": current.Metadata(),
			})
		}
		stats = h.agentManager.GetStats()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"items": agents,
		"stats": stats,
	})
}

func componentStatus(ready bool) string {
	if ready {
		return "ready"
	}
	return "missing"
}

func (h *SystemHandler) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, err error) {
	if err != nil && h.logger != nil {
		h.logger.Error(r.Context(), "system API error",
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

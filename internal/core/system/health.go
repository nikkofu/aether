package system

import (
	"encoding/json"
	"net/http"

	"github.com/nikkofu/aether/internal/runtime"
)

// HealthCheckHandler 提供系统的健康状态报告。
type HealthCheckHandler struct {
	rt *runtime.Runtime
}

func NewHealthCheckHandler(rt *runtime.Runtime) *HealthCheckHandler {
	return &HealthCheckHandler{rt: rt}
}

func (h *HealthCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status := "UP"
	details := make(map[string]string)

	// 1. 检查数据库
	if h.rt.Memory() == nil {
		status = "DOWN"
		details["database"] = "disconnected"
	} else {
		details["database"] = "connected"
	}

	// 2. 检查消息总线
	if h.rt.GetBus() == nil {
		status = "DOWN"
		details["bus"] = "uninitialized"
	} else {
		details["bus"] = "active"
	}

	// 3. 检查知识库
	if h.rt.KnowledgeGraph() == nil {
		details["knowledge_graph"] = "unavailable"
	} else {
		details["knowledge_graph"] = "ready"
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "DOWN" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(map[string]any{
		"status":  status,
		"details": details,
	})
}

// StartHealthServer 启动健康检查 HTTP 服务。
func StartHealthServer(addr string, rt *runtime.Runtime) error {
	http.Handle("/health", NewHealthCheckHandler(rt))
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	return http.ListenAndServe(addr, nil)
}

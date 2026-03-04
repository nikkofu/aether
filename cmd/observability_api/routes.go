package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/pkg/observability/graph"
	"github.com/nikkofu/aether/internal/pkg/observability/metrics"
	"github.com/nikkofu/aether/internal/pkg/observability/trace"
)

func setupRoutes(engine *trace.TraceEngine, metricsEngine *metrics.MetricsEngine) *http.ServeMux {
	mux := http.NewServeMux()

	// CORS handler wrapper
	withCORS := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h(w, r)
		}
	}

	mux.HandleFunc("/trace/", withCORS(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/trace/")
		if strings.HasSuffix(id, "/graph") {
			handleGetGraph(w, engine, strings.TrimSuffix(id, "/graph"))
			return
		}
		handleGetTrace(w, engine, id)
	}))

	mux.HandleFunc("/org/", withCORS(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/org/"), "/")
		if len(parts) < 2 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		orgID := parts[0]
		action := parts[1]

		switch action {
		case "recent_traces":
			handleRecentTraces(w, engine, orgID)
		case "metrics":
			handleOrgMetrics(w, metricsEngine, orgID)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	return mux
}

func handleGetTrace(w http.ResponseWriter, engine *trace.TraceEngine, id string) {
	t, err := engine.GetTrace(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(t)
}

func handleGetGraph(w http.ResponseWriter, engine *trace.TraceEngine, id string) {
	t, err := engine.GetTrace(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	dag := graph.BuildGraph(t)
	json.NewEncoder(w).Encode(dag)
}

func handleRecentTraces(w http.ResponseWriter, engine *trace.TraceEngine, orgID string) {
	traces, err := engine.GetRecentTraces(orgID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(traces)
}

func handleOrgMetrics(w http.ResponseWriter, engine *metrics.MetricsEngine, orgID string) {
	since := time.Now().Add(-24 * time.Hour) // 默认查询过去 24 小时
	m, err := engine.CalculateOrgMetrics(orgID, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(m)
}

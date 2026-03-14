package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nikkofu/aether/pkg/observability/metrics"
	"github.com/nikkofu/aether/pkg/observability/trace"
)

func TestSetupRoutesHealthz(t *testing.T) {
	t.Helper()

	storage, err := trace.NewSQLiteTraceStorage("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init trace storage: %v", err)
	}
	defer storage.GetDB().Close()

	engine := trace.NewTraceEngine(storage)
	metricsEngine := metrics.NewMetricsEngine(storage.GetDB())
	mux := setupRoutes(engine, metricsEngine, "/tmp/aether-rehearsal.db")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode health payload: %v", err)
	}

	if got := payload["status"]; got != "ok" {
		t.Fatalf("expected status ok, got %v", got)
	}
	if got := payload["database_path"]; got != "/tmp/aether-rehearsal.db" {
		t.Fatalf("expected database path to be returned, got %v", got)
	}
}

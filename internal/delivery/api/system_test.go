package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/pkg/bus"
)

type fakeAgent struct {
	name     string
	role     string
	status   agentdomain.Status
	metadata map[string]any
}

func (f *fakeAgent) Name() string { return f.name }
func (f *fakeAgent) Role() string { return f.role }
func (f *fakeAgent) Status() agentdomain.Status {
	return f.status
}
func (f *fakeAgent) Handle(ctx context.Context, msg agentdomain.Message) ([]agentdomain.Message, error) {
	return nil, nil
}
func (f *fakeAgent) Spawn(ctx context.Context, role string, payload map[string]any) (string, error) {
	return "", nil
}
func (f *fakeAgent) Shutdown(ctx context.Context) error { return nil }
func (f *fakeAgent) SetBus(b agentdomain.Bus)           {}
func (f *fakeAgent) SetStatus(s agentdomain.Status)     { f.status = s }
func (f *fakeAgent) Metadata() map[string]any           { return f.metadata }

type fakeAgentManager struct {
	agents []agentdomain.Agent
	stats  agentdomain.ManagerStats
}

func (f *fakeAgentManager) Register(a agentdomain.Agent) {}
func (f *fakeAgentManager) RegisterRole(role string, factory agentdomain.AgentRoleFactory) {
}
func (f *fakeAgentManager) Spawn(ctx context.Context, role string, payload map[string]any) (agentdomain.Agent, error) {
	return nil, nil
}
func (f *fakeAgentManager) List() []agentdomain.Agent                 { return f.agents }
func (f *fakeAgentManager) Get(name string) (agentdomain.Agent, bool) { return nil, false }
func (f *fakeAgentManager) GetStats() agentdomain.ManagerStats        { return f.stats }

func TestSystemAPI(t *testing.T) {
	manager := &fakeAgentManager{
		agents: []agentdomain.Agent{
			&fakeAgent{
				name:   "planner-1",
				role:   "planner",
				status: agentdomain.StatusRunning,
				metadata: map[string]any{
					"task_id": "task-123",
				},
			},
		},
		stats: agentdomain.ManagerStats{
			ActiveAgents:  1,
			TotalSpawns:   3,
			TotalFailures: 0,
			StatusCounts: map[agentdomain.Status]int{
				agentdomain.StatusRunning: 1,
			},
		},
	}

	handler := NewSystemHandler(nil, bus.NewMemoryBus(8), manager, &testLogger{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var payload map[string]any
		if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode health payload: %v", err)
		}

		if payload["status"] != "degraded" {
			t.Fatalf("expected degraded health, got %v", payload["status"])
		}
	})

	t.Run("agents", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var payload struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode agents payload: %v", err)
		}
		if len(payload.Items) != 1 {
			t.Fatalf("expected 1 agent, got %d", len(payload.Items))
		}
		if payload.Items[0]["name"] != "planner-1" {
			t.Fatalf("unexpected agent name: %v", payload.Items[0]["name"])
		}
	})
}

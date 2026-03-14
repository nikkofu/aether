package agent

import (
	"context"
	"testing"
	"time"
)

type subscribingBusStub struct {
	subscribed []string
}

func (b *subscribingBusStub) Publish(ctx context.Context, msg Message) {}

func (b *subscribingBusStub) Subscribe(a Agent) {
	b.subscribed = append(b.subscribed, a.Name())
}

type spawnedAgentStub struct {
	*BaseAgent
}

func (a *spawnedAgentStub) Handle(ctx context.Context, msg Message) ([]Message, error) {
	return nil, nil
}

func TestDefaultAgentManagerSpawnSubscribesDynamicAgents(t *testing.T) {
	busStub := &subscribingBusStub{}
	manager := NewDefaultAgentManager(nil, nil, nil, busStub, nil, 10, 10, nil)
	manager.RegisterRole("operational", func(ctx context.Context, name string, payload map[string]any) (Agent, error) {
		return &spawnedAgentStub{BaseAgent: NewBaseAgent(name, "operational")}, nil
	})

	agentInstance, err := manager.Spawn(context.Background(), "operational", map[string]any{
		"task_id": "task-1",
		"org_id":  "org-1",
		"at":      time.Now().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}
	if agentInstance == nil {
		t.Fatal("expected spawned agent")
	}
	if len(busStub.subscribed) != 1 || busStub.subscribed[0] != agentInstance.Name() {
		t.Fatalf("expected spawned agent to subscribe to bus, got %#v", busStub.subscribed)
	}
}

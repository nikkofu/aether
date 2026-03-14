package bus

import (
	"context"
	"testing"
	"time"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
)

type fakeTaskContextProvider struct {
	state map[string]context.Context
	stop  map[string]context.CancelFunc
}

func newFakeTaskContextProvider() *fakeTaskContextProvider {
	return &fakeTaskContextProvider{
		state: make(map[string]context.Context),
		stop:  make(map[string]context.CancelFunc),
	}
}

func (p *fakeTaskContextProvider) ContextForTask(parent context.Context, taskID string) (context.Context, context.CancelFunc) {
	if _, ok := p.state[taskID]; !ok {
		taskCtx, cancel := context.WithCancel(context.Background())
		p.state[taskID] = taskCtx
		p.stop[taskID] = cancel
	}

	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(p.state[taskID], cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (p *fakeTaskContextProvider) IsTaskCancelled(taskID string) bool {
	taskCtx, ok := p.state[taskID]
	if !ok {
		return false
	}
	return taskCtx.Err() != nil
}

func (p *fakeTaskContextProvider) Cancel(taskID string) {
	if cancel, ok := p.stop[taskID]; ok {
		cancel()
	}
}

type cancellableAgent struct {
	name      string
	started   chan struct{}
	cancelled chan struct{}
}

func (a *cancellableAgent) Name() string               { return a.name }
func (a *cancellableAgent) Role() string               { return "worker" }
func (a *cancellableAgent) Status() agentdomain.Status { return agentdomain.StatusRunning }
func (a *cancellableAgent) Spawn(ctx context.Context, role string, payload map[string]any) (string, error) {
	return "", nil
}
func (a *cancellableAgent) Shutdown(ctx context.Context) error { return nil }
func (a *cancellableAgent) SetBus(b agentdomain.Bus)           {}
func (a *cancellableAgent) SetStatus(s agentdomain.Status)     {}
func (a *cancellableAgent) Metadata() map[string]any           { return nil }

func (a *cancellableAgent) Handle(ctx context.Context, msg agentdomain.Message) ([]agentdomain.Message, error) {
	select {
	case <-a.started:
	default:
		close(a.started)
	}

	<-ctx.Done()
	close(a.cancelled)
	return nil, ctx.Err()
}

type delayedResponseAgent struct {
	name    string
	started chan struct{}
}

func (a *delayedResponseAgent) Name() string               { return a.name }
func (a *delayedResponseAgent) Role() string               { return "worker" }
func (a *delayedResponseAgent) Status() agentdomain.Status { return agentdomain.StatusRunning }
func (a *delayedResponseAgent) Spawn(ctx context.Context, role string, payload map[string]any) (string, error) {
	return "", nil
}
func (a *delayedResponseAgent) Shutdown(ctx context.Context) error { return nil }
func (a *delayedResponseAgent) SetBus(b agentdomain.Bus)           {}
func (a *delayedResponseAgent) SetStatus(s agentdomain.Status)     {}
func (a *delayedResponseAgent) Metadata() map[string]any           { return nil }

func (a *delayedResponseAgent) Handle(ctx context.Context, msg agentdomain.Message) ([]agentdomain.Message, error) {
	select {
	case <-a.started:
	default:
		close(a.started)
	}

	time.Sleep(100 * time.Millisecond)
	return []agentdomain.Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "cli",
		Type:      "work_progress",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id": msg.Payload["task_id"],
			"status":  "late_response",
		},
	}}, nil
}

type countingAgent struct {
	name string
	role string
	hits chan agentdomain.Message
}

func (a *countingAgent) Name() string               { return a.name }
func (a *countingAgent) Role() string               { return a.role }
func (a *countingAgent) Status() agentdomain.Status { return agentdomain.StatusRunning }
func (a *countingAgent) Spawn(ctx context.Context, role string, payload map[string]any) (string, error) {
	return "", nil
}
func (a *countingAgent) Shutdown(ctx context.Context) error { return nil }
func (a *countingAgent) SetBus(b agentdomain.Bus)           {}
func (a *countingAgent) SetStatus(s agentdomain.Status)     {}
func (a *countingAgent) Metadata() map[string]any           { return nil }

func (a *countingAgent) Handle(ctx context.Context, msg agentdomain.Message) ([]agentdomain.Message, error) {
	select {
	case a.hits <- msg:
	default:
	}
	return nil, nil
}

func TestMemoryBusTaskCancellation(t *testing.T) {
	t.Run("cancels active handler context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		b := NewMemoryBus(16)
		provider := newFakeTaskContextProvider()
		b.SetTaskContextProvider(provider)

		agent := &cancellableAgent{
			name:      "worker-ctx",
			started:   make(chan struct{}),
			cancelled: make(chan struct{}),
		}
		b.Subscribe(agent)
		go b.Start(ctx)

		b.Publish(ctx, agentdomain.Message{
			ID:        "task-ctx",
			From:      "api",
			To:        agent.name,
			Type:      "instruction",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"task_id": "task-ctx",
			},
		})

		select {
		case <-agent.started:
		case <-time.After(time.Second):
			t.Fatal("handler did not start")
		}

		provider.Cancel("task-ctx")

		select {
		case <-agent.cancelled:
		case <-time.After(time.Second):
			t.Fatal("handler context was not cancelled")
		}
	})

	t.Run("drops late responses for cancelled task", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		b := NewMemoryBus(16)
		provider := newFakeTaskContextProvider()
		b.SetTaskContextProvider(provider)

		agent := &delayedResponseAgent{
			name:    "worker-late",
			started: make(chan struct{}),
		}
		b.Subscribe(agent)

		progress := make(chan agentdomain.Message, 1)
		b.SubscribeToSubject(ctx, "*", func(msg agentdomain.Message) {
			if msg.Type == "work_progress" {
				progress <- msg
			}
		})

		go b.Start(ctx)

		b.Publish(ctx, agentdomain.Message{
			ID:        "task-late",
			From:      "api",
			To:        agent.name,
			Type:      "instruction",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"task_id": "task-late",
			},
		})

		select {
		case <-agent.started:
		case <-time.After(time.Second):
			t.Fatal("handler did not start")
		}

		provider.Cancel("task-late")

		select {
		case msg := <-progress:
			t.Fatalf("expected cancelled task response to be dropped, got %s", msg.Type)
		case <-time.After(250 * time.Millisecond):
		}
	})
}

func TestMemoryBusRoutesOnlyToExplicitRecipient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := NewMemoryBus(16)
	supervisor := &countingAgent{
		name: "supervisor",
		role: "supervisor",
		hits: make(chan agentdomain.Message, 1),
	}
	coder := &countingAgent{
		name: "coder",
		role: "coder",
		hits: make(chan agentdomain.Message, 1),
	}

	b.Subscribe(supervisor)
	b.Subscribe(coder)
	go b.Start(ctx)

	b.Publish(ctx, agentdomain.Message{
		ID:        "task-routing",
		From:      "planner",
		To:        "coder",
		Type:      "instruction",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"task_id": "task-routing",
		},
	})

	select {
	case <-coder.hits:
	case <-time.After(time.Second):
		t.Fatal("expected explicit recipient to receive the message")
	}

	select {
	case msg := <-supervisor.hits:
		t.Fatalf("did not expect supervisor to receive %s routed to coder", msg.Type)
	case <-time.After(200 * time.Millisecond):
	}
}

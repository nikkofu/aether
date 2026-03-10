package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/pkg/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// NATSBus 实现了具备分布式故障恢复的消息总线。
type NATSBus struct {
	conn                *nats.Conn
	subs                []*nats.Subscription
	mu                  sync.Mutex
	logger              logging.Logger
	taskContextProvider TaskContextProvider
}

func NewNATSBus(url string) (*NATSBus, error) {
	nc, err := nats.Connect(url, nats.Name("Aether Bus"), nats.MaxReconnects(-1))
	if err != nil {
		return nil, fmt.Errorf("无法连接到 NATS: %w", err)
	}
	return &NATSBus{conn: nc}, nil
}

func (b *NATSBus) SetLogger(l logging.Logger) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logger = l
}

func (b *NATSBus) SetTaskContextProvider(provider TaskContextProvider) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.taskContextProvider = provider
}

func (b *NATSBus) Publish(ctx context.Context, msg agent.Message) {
	if msg.To == "" {
		msg.To = "broadcast"
	}
	subject := fmt.Sprintf("aether.agent.%s", msg.To)

	// OpenTelemetry Context Propagation: Inject
	if msg.Header == nil {
		msg.Header = make(map[string]string)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(msg.Header))

	data, _ := json.Marshal(msg)
	_ = b.conn.Publish(subject, data)
}

func (b *NATSBus) SubscribeToSubject(ctx context.Context, subject string, handler func(msg agent.Message)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	natsSubject := fmt.Sprintf("aether.agent.%s", subject)
	if subject == "*" {
		natsSubject = "aether.agent.>"
	}

	h := func(m *nats.Msg) {
		var msg agent.Message
		if err := json.Unmarshal(m.Data, &msg); err != nil {
			return
		}
		if b.isTaskCancelled(taskIDFromMessage(msg)) {
			return
		}
		handler(msg)
	}

	sub, _ := b.conn.Subscribe(natsSubject, h)
	b.subs = append(b.subs, sub)
}

func (b *NATSBus) Subscribe(a agent.Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	a.SetBus(b)
	subject := fmt.Sprintf("aether.agent.%s", a.Name())

	handler := func(m *nats.Msg) {
		var msg agent.Message
		_ = json.Unmarshal(m.Data, &msg)
		msgTaskID := taskIDFromMessage(msg)
		if b.isTaskCancelled(msgTaskID) {
			return
		}

		// 故障恢复：Panic Recovery
		defer func() {
			if r := recover(); r != nil {
				if b.logger != nil {
					b.logger.Error(context.Background(), "分布式代理崩溃拦截",
						logging.String("agent", a.Name()),
						logging.Any("panic", r),
						logging.String("stack", string(debug.Stack())),
					)
				}
				b.Publish(context.Background(), agent.Message{
					ID:        msg.ID,
					From:      a.Name(),
					To:        "supervisor",
					Type:      "system.alert",
					Timestamp: time.Now(),
					Payload: map[string]any{
						"severity":  "CRITICAL",
						"message":   "Panic occurred",
						"origin_id": msg.ID,
						"task_id":   msg.ID,
					},
				})
			}
		}()

		// OpenTelemetry Context Propagation: Extract
		ctx := context.Background()
		if msg.Header != nil {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(msg.Header))
		}

		baseCtx, release := b.contextForTask(ctx, msgTaskID)
		defer release()

		// 故障恢复：Context 超时传播 (15分钟分布式执行上限)
		ctx, cancel := context.WithTimeout(baseCtx, 15*time.Minute)
		defer cancel()

		responses, err := a.Handle(ctx, msg)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) || b.isTaskCancelled(msgTaskID) {
				if b.logger != nil {
					b.logger.Info(context.Background(), "分布式任务执行已取消",
						logging.String("agent", a.Name()),
						logging.String("task_id", msgTaskID),
					)
				}
				return
			}
			if b.logger != nil {
				b.logger.Error(ctx, "分布式代理处理失败", logging.String("agent", a.Name()), logging.Err(err))
			}
			b.Publish(ctx, agent.Message{
				ID:        msg.ID,
				From:      a.Name(),
				To:        "supervisor",
				Type:      "system.alert",
				Timestamp: time.Now(),
				Payload: map[string]any{
					"severity":  "HIGH",
					"message":   err.Error(),
					"origin_id": msg.ID,
					"task_id":   msg.ID,
				},
			})
			return
		}

		for _, resp := range responses {
			if b.isTaskCancelled(msgTaskID) || b.isTaskCancelled(taskIDFromMessage(resp)) {
				return
			}
			b.Publish(ctx, resp)
		}
	}

	sub, _ := b.conn.Subscribe(subject, handler)
	b.subs = append(b.subs, sub)
}

func (b *NATSBus) WaitReady(ctx context.Context) error {
	if b.conn == nil || !b.conn.IsConnected() {
		return fmt.Errorf("NATS 未连接")
	}
	// 简单的 ping 检查
	return b.conn.FlushWithContext(ctx)
}

func (b *NATSBus) Start(ctx context.Context) {
	<-ctx.Done()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.subs {
		s.Unsubscribe()
	}
	b.conn.Close()
}

func (b *NATSBus) contextForTask(parent context.Context, taskID string) (context.Context, func()) {
	b.mu.Lock()
	provider := b.taskContextProvider
	b.mu.Unlock()

	if provider == nil || taskID == "" {
		return parent, func() {}
	}
	return provider.ContextForTask(parent, taskID)
}

func (b *NATSBus) isTaskCancelled(taskID string) bool {
	b.mu.Lock()
	provider := b.taskContextProvider
	b.mu.Unlock()

	if provider == nil || taskID == "" {
		return false
	}
	return provider.IsTaskCancelled(taskID)
}

var _ Bus = (*NATSBus)(nil)

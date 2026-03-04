package bus

import (
	"context"
	"encoding/json"
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
	conn   *nats.Conn
	subs   []*nats.Subscription
	mu     sync.Mutex
	logger logging.Logger
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
					From:      a.Name(),
					To:        "supervisor",
					Type:      "system.alert",
					Timestamp: time.Now(),
					Payload:   map[string]any{"severity": "CRITICAL", "message": "Panic occurred"},
				})
			}
		}()

		var msg agent.Message
		json.Unmarshal(m.Data, &msg)

		// OpenTelemetry Context Propagation: Extract
		ctx := context.Background()
		if msg.Header != nil {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(msg.Header))
		}

		// 故障恢复：Context 超时传播 (120s 分布式执行上限)
		ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		responses, err := a.Handle(ctx, msg)
		if err != nil {
			if b.logger != nil {
				b.logger.Error(ctx, "分布式代理处理失败", logging.String("agent", a.Name()), logging.Err(err))
			}
			b.Publish(ctx, agent.Message{
				From:      a.Name(),
				To:        "supervisor",
				Type:      "system.alert",
				Timestamp: time.Now(),
				Payload:   map[string]any{"severity": "HIGH", "message": err.Error()},
			})
			return
		}

		for _, resp := range responses {
			b.Publish(ctx, resp)
		}
	}

	sub, _ := b.conn.Subscribe(subject, handler)
	b.subs = append(b.subs, sub)
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

var _ Bus = (*NATSBus)(nil)

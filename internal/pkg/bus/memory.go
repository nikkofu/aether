package bus

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/core/agent"
	"github.com/nikkofu/aether/internal/pkg/logging"
)

type subjectSub struct {
	subject string
	handler func(msg agent.Message)
}

// MemoryBus 实现了具备故障恢复能力的内存消息总线。
type MemoryBus struct {
	mu           sync.RWMutex
	subscribers  []agent.Agent
	subjectSubs  []subjectSub
	queue        chan agent.Message
	logger       logging.Logger
}

func (b *MemoryBus) SubscribeToSubject(ctx context.Context, subject string, handler func(msg agent.Message)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subjectSubs = append(b.subjectSubs, subjectSub{subject: subject, handler: handler})
}

func NewMemoryBus(bufferSize int) *MemoryBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &MemoryBus{
		queue: make(chan agent.Message, bufferSize),
	}
}

func (b *MemoryBus) SetLogger(l logging.Logger) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logger = l
}

func (b *MemoryBus) Publish(ctx context.Context, msg agent.Message) {
	b.queue <- msg
}

func (b *MemoryBus) Subscribe(a agent.Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	a.SetBus(b)
	b.subscribers = append(b.subscribers, a)
}

func (b *MemoryBus) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-b.queue:
			b.dispatch(ctx, msg)
		}
	}
}

func (b *MemoryBus) dispatch(ctx context.Context, msg agent.Message) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subjectSubs {
		if msg.To == sub.subject || sub.subject == "*" {
			go sub.handler(msg)
		}
	}

	for _, sub := range b.subscribers {
		if msg.To != "" && msg.To != sub.Name() {
			continue
		}

		// 为每个代理启动受保护的协程
		go func(a agent.Agent, m agent.Message) {
			// 核心需求：Panic Recovery
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					err := fmt.Errorf("代理 %s 发生崩溃 (Panic): %v", a.Name(), r)
					
					if b.logger != nil {
						b.logger.Error(ctx, "代理崩溃拦截",
							logging.String("agent", a.Name()),
							logging.Any("panic", r),
							logging.String("stack", string(stack)),
						)
					}

					// 发布系统告警以便 Supervisor 决策
					b.Publish(ctx, agent.Message{
						From:      a.Name(),
						To:        "supervisor",
						Type:      "system.alert",
						Timestamp: time.Now(),
						Payload: map[string]any{
							"severity": "CRITICAL",
							"message":  err.Error(),
							"panic":    true,
						},
					})
				}
			}()

			// 核心需求：超时取消 (默认 60s 处理限制)
			handleCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			responses, err := a.Handle(handleCtx, m)
			if err != nil {
				if b.logger != nil {
					b.logger.Error(ctx, "代理处理消息失败", logging.String("agent", a.Name()), logging.Err(err))
				}
				// 失败也上报给 Supervisor 触发重试逻辑
				b.Publish(ctx, agent.Message{
					From:      a.Name(),
					To:        "supervisor",
					Type:      "system.alert",
					Timestamp: time.Now(),
					Payload:   map[string]any{"severity": "HIGH", "message": err.Error(), "error": err.Error()},
				})
				return
			}

			for _, resp := range responses {
				b.Publish(ctx, resp)
			}
		}(sub, msg)
	}
}

var _ Bus = (*MemoryBus)(nil)

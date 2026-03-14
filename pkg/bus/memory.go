package bus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/pkg/logging"
)

type subjectSub struct {
	subject string
	handler func(msg agent.Message)
}

// MemoryBus 实现了具备故障恢复能力的内存消息总线。
type MemoryBus struct {
	mu                  sync.RWMutex
	subscribers         []agent.Agent
	subjectSubs         []subjectSub
	queue               chan agent.Message
	logger              logging.Logger
	taskContextProvider TaskContextProvider
}

func (b *MemoryBus) SubscribeToSubject(ctx context.Context, subject string, handler func(msg agent.Message)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subjectSubs = append(b.subjectSubs, subjectSub{subject: subject, handler: handler})
}

func (b *MemoryBus) WaitReady(ctx context.Context) error {
	// 对于内存总线，主要确保 queue 已初始化且有处理能力
	// 我们通过发送一个 ping 消息并等待它被 dispatch 来模拟就绪
	if b.logger != nil {
		b.logger.Debug(ctx, "正在等待 MemoryBus 就绪...")
	}

	// 给一点极短的固定延迟，确保所有订阅 Goroutine 已经跑起来
	select {
	case <-time.After(100 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func NewMemoryBus(bufferSize int) *MemoryBus {
	if bufferSize <= 0 {
		bufferSize = 10000 // 企业级高并发缓冲区
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

func (b *MemoryBus) SetTaskContextProvider(provider TaskContextProvider) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.taskContextProvider = provider
}

func (b *MemoryBus) Publish(ctx context.Context, msg agent.Message) {
	if os.Getenv("AETHER_LOG_LEVEL") == "debug" {
		fmt.Fprintf(os.Stderr, ">> [BUS] 📥 消息入队: %s -> %s\n", msg.From, msg.To)
	}
	// 增加非阻塞尝试，如果队列真满了则报错，而不是死锁
	select {
	case b.queue <- msg:
	default:
		fmt.Fprintf(os.Stderr, "❌ [BUS] 严重警告: 消息队列溢出，丢弃来自 %s 的消息!\n", msg.From)
	}
}

func (b *MemoryBus) Subscribe(a agent.Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	a.SetBus(b)
	b.subscribers = append(b.subscribers, a)
	if os.Getenv("AETHER_LOG_LEVEL") == "debug" {
		fmt.Fprintf(os.Stderr, "✅ [BUS] 代理注册成功: %s (%s)\n", a.Name(), a.Role())
	}
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
	if os.Getenv("AETHER_LOG_LEVEL") == "debug" {
		fmt.Fprintf(os.Stderr, ">> [BUS] 📢 消息流转: %s -> %s (类型: %s)\n", msg.From, msg.To, msg.Type)
	}

	taskID := taskIDFromMessage(msg)
	if b.isTaskCancelled(taskID) {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subjectSubs {
		if msg.To == sub.subject || sub.subject == "*" {
			go sub.handler(msg)
		}
	}

	for _, sub := range b.subscribers {
		// 同一 Agent 发出的消息不再回投给自己，避免噪音和自循环。
		if msg.From == sub.Name() {
			continue
		}

		// 总线只负责显式路由，不再在这里隐式授予编排特权。
		if msg.To != "" && msg.To != sub.Name() {
			continue
		}

		// 为每个代理启动受保护的协程
		go func(a agent.Agent, m agent.Message) {
			msgTaskID := taskIDFromMessage(m)

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
						ID:        m.ID,
						From:      a.Name(),
						To:        "supervisor",
						Type:      "system.alert",
						Timestamp: time.Now(),
						Payload: map[string]any{
							"severity":  "CRITICAL",
							"message":   err.Error(),
							"panic":     true,
							"origin_id": m.ID,
							"task_id":   m.ID,
						},
					})
				}
			}()

			baseCtx, release := b.contextForTask(ctx, msgTaskID)
			defer release()

			// 核心需求：超时取消 (企业级长任务处理限制)
			handleCtx, cancel := context.WithTimeout(baseCtx, 15*time.Minute)
			defer cancel()

			if err := handleCtx.Err(); err != nil {
				return
			}

			responses, err := a.Handle(handleCtx, m)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(handleCtx.Err(), context.Canceled) || b.isTaskCancelled(msgTaskID) {
					if b.logger != nil {
						b.logger.Info(ctx, "任务执行已取消", logging.String("agent", a.Name()), logging.String("task_id", msgTaskID))
					}
					return
				}
				if b.logger != nil {
					b.logger.Error(ctx, "代理处理消息失败", logging.String("agent", a.Name()), logging.Err(err))
				}
				// 失败也上报给 Supervisor 触发重试逻辑
				b.Publish(ctx, agent.Message{
					ID:        m.ID,
					From:      a.Name(),
					To:        "supervisor",
					Type:      "system.alert",
					Timestamp: time.Now(),
					Payload: map[string]any{
						"severity":  "HIGH",
						"message":   err.Error(),
						"error":     err.Error(),
						"origin_id": m.ID,
						"task_id":   m.ID,
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
		}(sub, msg)
	}
}

func (b *MemoryBus) contextForTask(parent context.Context, taskID string) (context.Context, func()) {
	b.mu.RLock()
	provider := b.taskContextProvider
	b.mu.RUnlock()

	if provider == nil || taskID == "" {
		return parent, func() {}
	}
	return provider.ContextForTask(parent, taskID)
}

func (b *MemoryBus) isTaskCancelled(taskID string) bool {
	b.mu.RLock()
	provider := b.taskContextProvider
	b.mu.RUnlock()

	if provider == nil || taskID == "" {
		return false
	}
	return provider.IsTaskCancelled(taskID)
}

func taskIDFromMessage(msg agent.Message) string {
	if msg.Payload != nil {
		if taskID, ok := msg.Payload["task_id"].(string); ok && taskID != "" {
			return taskID
		}
	}
	return msg.ID
}

var _ Bus = (*MemoryBus)(nil)

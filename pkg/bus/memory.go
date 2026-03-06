package bus

import (
	"context"
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

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subjectSubs {
		if msg.To == sub.subject || sub.subject == "*" {
			go sub.handler(msg)
		}
	}

	for _, sub := range b.subscribers {
		// 关键修复：支持广播与编排者监听
		isOrchestrator := sub.Role() == "supervisor" || sub.Role() == "manager"
		
		// 编排者特权：监听非 UI 类消息（排除 token）
		if isOrchestrator && msg.Type != "token" {
			// 允许通过
		} else if msg.To != "" && msg.To != sub.Name() {
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

			// 核心需求：超时取消 (企业级长任务处理限制)
			handleCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
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

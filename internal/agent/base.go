package agent

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/learning"
	"github.com/nikkofu/aether/internal/logging"
	"github.com/nikkofu/aether/internal/reflection"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// BaseAgent 提供了 Agent 接口的基础实现及自进化闭环。
type BaseAgent struct {
	mu       sync.RWMutex
	name     string
	role     string
	status   Status
	bus      Bus
	metadata map[string]any

	// 自进化组件
	reflector      reflection.Reflector
	reflectionStore reflection.Store
	learningEngine *learning.LearningEngine
	logger         logging.Logger
}

func NewBaseAgent(name, role string) *BaseAgent {
	return &BaseAgent{
		name:     name,
		role:     role,
		status:   StatusIdle,
		metadata: make(map[string]any),
	}
}

// SetComponents 注入核心组件。
func (b *BaseAgent) SetComponents(ref reflection.Reflector, res reflection.Store, le *learning.LearningEngine, l logging.Logger) {
	b.reflector = ref
	b.reflectionStore = res
	b.learningEngine = le
	b.logger = l
}

func (b *BaseAgent) Name() string { return b.name }
func (b *BaseAgent) Role() string { return b.role }

func (b *BaseAgent) Status() Status {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func (b *BaseAgent) SetStatus(s Status) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = s
}

func (b *BaseAgent) SetBus(bus Bus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bus = bus
}

func (b *BaseAgent) Metadata() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make(map[string]any)
	for k, v := range b.metadata { res[k] = v }
	return res
}

// ProtectedHandle 是一个包装器，增加了 Panic Recovery 和反思闭环。
func (b *BaseAgent) ProtectedHandle(ctx context.Context, msg Message, handler func() ([]Message, error)) (res []Message, err error) {
	// Tracing: agent execution
	tracer := otel.Tracer("aether-tracer")
	ctx, span := tracer.Start(ctx, "agent.execute")
	span.SetAttributes(
		attribute.String("agent.name", b.name),
		attribute.String("agent.role", b.role),
		attribute.String("msg.id", msg.ID),
		attribute.String("msg.type", msg.Type),
	)
	defer span.End()

	b.SetStatus(StatusRunning)
	start := time.Now()

	// 1. Panic Recovery
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in agent %s: %v", b.name, r)
			if b.logger != nil {
				b.logger.Error(ctx, "代理崩溃拦截", logging.Any("panic", r), logging.String("stack", string(debug.Stack())))
			}
			b.SetStatus(StatusFailed)
			span.RecordError(err)
			span.SetStatus(codes.Error, "panic occurred")
		} else if err != nil {
			b.SetStatus(StatusFailed)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			b.SetStatus(StatusIdle)
			span.SetStatus(codes.Ok, "success")
		}

		// 2. 执行结束后触发反思闭环
		go b.reflectAndLearn(ctx, msg.ID, start, err)
	}()

	return handler()
}

func (b *BaseAgent) reflectAndLearn(ctx context.Context, taskID string, start time.Time, err error) {
	if b.reflector == nil || b.learningEngine == nil { return }

	input := reflection.ReflectionInput{
		AgentName: b.name,
		TaskID:    taskID,
		Error:     err,
		Duration:  time.Since(start),
		// Cost 通常需要从 Skill 返回的消息中提取，这里简化处理
	}

	// 执行反思
	reflectResult, rErr := b.reflector.Reflect(ctx, input)
	if rErr != nil { return }

	// 保存反思
	if b.reflectionStore != nil {
		_ = b.reflectionStore.Save(ctx, reflectResult)
	}

	// 更新策略
	_ = b.learningEngine.UpdateStrategy(reflectResult)
}

func (b *BaseAgent) Spawn(ctx context.Context, role string, payload map[string]any) (string, error) { return "", nil }
func (b *BaseAgent) Shutdown(ctx context.Context) error { return nil }

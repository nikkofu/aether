package agent

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/nikkofu/aether/pkg/logging"
	go_otel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// BaseAgent 是所有具体代理实现的基类，提供了基础的状态管理和元数据支持。
type BaseAgent struct {
	name     string
	role     string
	status   Status
	metadata map[string]any
	logger   logging.Logger
	bus      Bus // 增加对 Bus 的支持以符合接口
}

func NewBaseAgent(name, role string) *BaseAgent {
	return &BaseAgent{
		name:     name,
		role:     role,
		status:   StatusIdle,
		metadata: make(map[string]any),
	}
}

func (b *BaseAgent) Name() string { return b.name }
func (b *BaseAgent) Role() string { return b.role }

func (b *BaseAgent) Status() Status { return b.status }
func (b *BaseAgent) SetStatus(s Status) { b.status = s }

func (b *BaseAgent) SetLogger(l logging.Logger) { b.logger = l }
func (b *BaseAgent) SetBus(bus Bus) { b.bus = bus }

func (b *BaseAgent) Metadata() map[string]any {
	res := make(map[string]any)
	for k, v := range b.metadata { res[k] = v }
	return res
}

// ProtectedHandle 是一个包装器，增加了身份标识、Panic Recovery 和故障分析。
func (b *BaseAgent) ProtectedHandle(ctx context.Context, msg Message, handler func() ([]Message, error)) (res []Message, err error) {
	// 注入身份标识到 Context
	ctx = context.WithValue(ctx, "agent_name", b.name)
	ctx = context.WithValue(ctx, "agent_role", b.role)

	// Tracing: 使用 go_otel
	tracer := go_otel.Tracer("aether-tracer")
	ctx, span := tracer.Start(ctx, fmt.Sprintf("[%s:%s] Handle", strings.ToUpper(b.role), b.name))
	span.SetAttributes(
		attribute.String("agent.id", b.name),
		attribute.String("agent.role", b.role),
		attribute.String("msg.type", msg.Type),
	)
	defer span.End()

	b.SetStatus(StatusRunning)

	// 1. Panic Recovery
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in agent %s: %v", b.name, r)
			if b.logger != nil {
				b.logger.Error(ctx, "Agent 发生崩溃", logging.Any("panic", r), logging.String("stack", string(debug.Stack())))
			}
			b.SetStatus(StatusFailed)
			span.RecordError(err)
		} else if err != nil {
			b.SetStatus(StatusFailed)
			span.RecordError(err)
		} else {
			b.SetStatus(StatusIdle)
		}
	}()

	return handler()
}

func (b *BaseAgent) HandleSystemMessage(ctx context.Context, msg Message) []Message { return nil }
func (b *BaseAgent) Spawn(ctx context.Context, role string, payload map[string]any) (string, error) { return "", nil }
func (b *BaseAgent) Shutdown(ctx context.Context) error { return nil }

package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TraceEngine 是基于 OpenTelemetry 的链路追踪引擎。
// 它封装了标准的 otel.Tracer，并提供面向业务层级的追踪接口。
type TraceEngine struct {
	tracer  trace.Tracer
	storage TraceStorage
}

// NewTraceEngine 创建一个新的追踪引擎实例。
func NewTraceEngine(storage TraceStorage) *TraceEngine {
	return &TraceEngine{
		tracer:  otel.Tracer("aether-tracer"),
		storage: storage,
	}
}

// StartSpan 在给定的 Context 下开始一个新的操作跨度。
// 它利用 OTel 的 Context 传播机制自动处理父子关系。
func (e *TraceEngine) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return e.tracer.Start(ctx, name, opts...)
}

// StartTrace 为业务起始点创建一个根追踪。
// 建议在 Strategic 层级调用。
func (e *TraceEngine) StartTrace(ctx context.Context, name string, orgID string) (context.Context, trace.Span) {
	opts := []trace.SpanStartOption{
		trace.WithAttributes(attribute.String("org_id", orgID)),
		trace.WithNewRoot(),
	}
	return e.tracer.Start(ctx, name, opts...)
}

// RecordEvent 保持向后兼容，但建议直接使用 OTel Span 的 AddEvent 方法。
func (e *TraceEngine) RecordEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// GetTrace 仍然从现有的存储层获取历史追踪信息。
func (e *TraceEngine) GetTrace(traceID string) (*Trace, error) {
	return e.storage.GetTrace(traceID)
}

// GetRecentTraces 获取指定组织的最近追踪记录。
func (e *TraceEngine) GetRecentTraces(orgID string, limit int) ([]*Trace, error) {
	return e.storage.GetRecentTraces(orgID, limit)
}

// SetAttributes 为当前 Context 中的 Span 设置属性。
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

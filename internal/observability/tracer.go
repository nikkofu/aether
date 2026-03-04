package observability

import (
	"context"
)

// contextKey 用于在 context 中安全存储追踪信息。
type contextKey string

const (
	TraceKey contextKey = "aether_trace_id"
	SpanKey  contextKey = "aether_parent_span_id"
	DepthKey contextKey = "aether_trace_depth"
)

// Tracer 定义了系统观测性的追踪接口。
type Tracer interface {
	// StartSpan 开始一个新的追踪段。
	StartSpan(ctx context.Context, name string, meta map[string]any) (context.Context, Span)
}

// Span 代表追踪链路中的一个原子操作段。
type Span interface {
	// End 结束当前 Span 的记录。
	End()
}

package observability

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// ConsoleRenderer 实现了 Tracer 接口，并以可视化的树状结构输出追踪信息。
type ConsoleRenderer struct {
	mu sync.Mutex
}

// NewConsoleRenderer 创建一个新的控制台渲染器。
func NewConsoleRenderer() *ConsoleRenderer {
	return &ConsoleRenderer{}
}

// StartSpan 开始一个新的追踪段并计算深度。
func (r *ConsoleRenderer) StartSpan(ctx context.Context, name string, meta map[string]any) (context.Context, Span) {
	// 1. 获取或生成 TraceID (优先使用 OpenTelemetry)
	var traceID string
	if otelSpan := trace.SpanFromContext(ctx); otelSpan.SpanContext().IsValid() {
		traceID = otelSpan.SpanContext().TraceID().String()
	} else if id, ok := ctx.Value(TraceKey).(string); ok {
		traceID = id
	} else {
		traceID = strings.ReplaceAll(uuid.New().String(), "-", "")
	}

	// 2. 计算深度
	depth, _ := ctx.Value(DepthKey).(int)

	// 3. 生成当前 SpanID
	spanID := strings.ReplaceAll(uuid.New().String(), "-", "")[:4]

	s := &consoleSpan{
		traceID:   traceID,
		spanID:    spanID,
		name:      name,
		meta:      meta,
		depth:     depth,
		startTime: time.Now(),
		renderer:  r,
	}

	// 4. 注入 Context 并增加深度
	newCtx := context.WithValue(ctx, TraceKey, traceID)
	newCtx = context.WithValue(newCtx, SpanKey, spanID)
	newCtx = context.WithValue(newCtx, DepthKey, depth+1)

	return newCtx, s
}

type consoleSpan struct {
	traceID   string
	spanID    string
	name      string
	meta      map[string]any
	depth     int
	startTime time.Time
	renderer  *ConsoleRenderer
}

// End 格式化并打印树状追踪日志。
func (s *consoleSpan) End() {
	duration := time.Since(s.startTime)
	
	s.renderer.mu.Lock()
	defer s.renderer.mu.Unlock()

	// 构建前缀
	indent := strings.Repeat("  ", s.depth)
	treeSymbol := "└── "
	if s.depth == 0 {
		treeSymbol = "● "
	}

	// 格式化输出
	displayID := s.traceID
	if len(displayID) > 8 {
		displayID = displayID[:8]
	}

	fmt.Fprintf(os.Stderr, "[%s] %s%s%-25s (%s)\n",
		displayID,
		indent,
		treeSymbol,
		s.name,
		duration.Round(time.Millisecond),
	)

	// 如果开启了 Debug 级别，打印详细的元数据
	if os.Getenv("AETHER_LOG_LEVEL") == "debug" && len(s.meta) > 0 {
		for k, v := range s.meta {
			// 对大文本进行缩进处理，使其更易读
			valStr := fmt.Sprintf("%v", v)
			if strings.Contains(valStr, "\n") {
				valStr = "\n" + indent + "      " + strings.ReplaceAll(valStr, "\n", "\n"+indent+"      ")
			}
			fmt.Fprintf(os.Stderr, "%s    | %s: %s\n", indent, k, valStr)
		}
	}
}

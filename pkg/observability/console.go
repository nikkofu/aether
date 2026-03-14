package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	obstrace "github.com/nikkofu/aether/pkg/observability/trace"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ConsoleRenderer 实现了 Tracer 接口，并以可视化的树状结构输出追踪信息。
type ConsoleRenderer struct {
	mu      sync.Mutex
	storage obstrace.TraceStorage
}

// NewConsoleRenderer 创建一个新的控制台渲染器。
func NewConsoleRenderer() *ConsoleRenderer {
	return &ConsoleRenderer{}
}

// SetTraceStorage configures optional SQLite-backed trace persistence.
func (r *ConsoleRenderer) SetTraceStorage(storage obstrace.TraceStorage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storage = storage
}

// StartSpan 开始一个新的追踪段并计算深度。
func (r *ConsoleRenderer) StartSpan(ctx context.Context, name string, meta map[string]any) (context.Context, Span) {
	// 1. 获取或生成 TraceID (优先使用 OpenTelemetry)
	var traceID string
	if otelSpan := oteltrace.SpanFromContext(ctx); otelSpan.SpanContext().IsValid() {
		traceID = otelSpan.SpanContext().TraceID().String()
	} else if id, ok := ctx.Value(TraceKey).(string); ok {
		traceID = id
	} else {
		traceID = strings.ReplaceAll(uuid.New().String(), "-", "")
	}

	// 2. 计算深度和父跨度
	depth, _ := ctx.Value(DepthKey).(int)
	parentSpanID, _ := ctx.Value(SpanKey).(string)
	startedAt := time.Now()
	otelCtx, otelSpan := otel.Tracer("aether-console").Start(ctx, name)

	// 3. 生成当前 SpanID
	spanID := strings.ReplaceAll(uuid.New().String(), "-", "")[:4]

	if otelSpan.SpanContext().IsValid() {
		traceID = otelSpan.SpanContext().TraceID().String()
	}

	s := &consoleSpan{
		traceID:      traceID,
		spanID:       spanID,
		parentSpanID: parentSpanID,
		name:         name,
		meta:         meta,
		depth:        depth,
		startTime:    startedAt,
		renderer:     r,
		otelSpan:     otelSpan,
	}
	s.persist(startedAt)

	// 4. 注入 Context 并增加深度
	newCtx := context.WithValue(otelCtx, TraceKey, traceID)
	newCtx = context.WithValue(newCtx, SpanKey, spanID)
	newCtx = context.WithValue(newCtx, DepthKey, depth+1)

	return newCtx, s
}

type consoleSpan struct {
	traceID       string
	spanID        string
	parentSpanID  string
	name          string
	meta          map[string]any
	depth         int
	startTime     time.Time
	renderer      *ConsoleRenderer
	persistedSpan *obstrace.Span
	otelSpan      oteltrace.Span
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

	// 格式化输出标题
	displayID := s.traceID
	if len(displayID) > 8 {
		displayID = displayID[:8]
	}

	fmt.Fprintf(os.Stderr, "\n[%s] %s%s %s (%s)\n",
		displayID,
		indent,
		treeSymbol,
		strings.ToUpper(s.name),
		duration.Round(time.Millisecond),
	)

	// 如果开启了 Debug 级别，打印详细的元数据
	if os.Getenv("AETHER_LOG_LEVEL") == "debug" && len(s.meta) > 0 {
		for k, v := range s.meta {
			valStr := fmt.Sprintf("%v", v)
			// 如果是 JSON 字符串，尝试美化
			if strings.HasPrefix(valStr, "{") || strings.HasPrefix(valStr, "[") {
				var prettyJSON bytes.Buffer
				if err := json.Indent(&prettyJSON, []byte(valStr), indent+"      ", "  "); err == nil {
					valStr = prettyJSON.String()
				}
			}

			// 处理多行缩进
			if strings.Contains(valStr, "\n") {
				valStr = "\n" + indent + "      " + strings.ReplaceAll(valStr, "\n", "\n"+indent+"      ")
			}

			fmt.Fprintf(os.Stderr, "%s    ├── %s: %s\n", indent, strings.ToUpper(k), valStr)
		}
		fmt.Fprintf(os.Stderr, "%s    └── [END META]\n", indent)
	}

	if s.persistedSpan != nil && s.renderer.storage != nil {
		s.persistedSpan.End("ok")
		_ = s.renderer.storage.UpdateSpan(s.persistedSpan)
	}
	if s.otelSpan != nil {
		s.otelSpan.End()
	}
}

func (s *consoleSpan) persist(startedAt time.Time) {
	if s.renderer == nil || s.renderer.storage == nil {
		return
	}

	record := obstrace.NewSpan(s.traceID, s.parentSpanID, s.name, resolveTraceLayer(s.meta))
	record.SpanID = s.spanID
	record.StartedAt = startedAt
	record.Metadata = cloneMeta(s.meta)
	record.OrgID = resolveTraceOrgID(s.meta)
	record.AgentID = resolveTraceAgentID(s.meta)
	s.persistedSpan = record

	if s.parentSpanID == "" {
		root := &obstrace.Trace{
			ID:        s.traceID,
			OrgID:     record.OrgID,
			StartedAt: startedAt,
			Spans:     make([]*obstrace.Span, 0),
		}
		_ = s.renderer.storage.InsertTrace(root)
	}

	_ = s.renderer.storage.InsertSpan(record)
}

func resolveTraceOrgID(meta map[string]any) string {
	if value, ok := meta["org_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "default"
}

func resolveTraceAgentID(meta map[string]any) string {
	for _, key := range []string{"agent_id", "agent_name", "agent"} {
		if value, ok := meta[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveTraceLayer(meta map[string]any) string {
	for _, key := range []string{"layer", "role"} {
		if value, ok := meta[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "runtime"
}

func cloneMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return map[string]any{}
	}

	cloned := make(map[string]any, len(meta))
	for key, value := range meta {
		cloned[key] = value
	}
	return cloned
}

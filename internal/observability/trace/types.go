package trace

import (
	"time"

	"github.com/google/uuid"
)

// ExecutionEvent 代表链路追踪系统中的原子事件。
// 该结构体优化了 JSON 序列化，适用于存储、审计和跨组件异步消息传递。
type ExecutionEvent struct {
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	OrgID        string         `json:"org_id,omitempty"`
	AgentID      string         `json:"agent_id,omitempty"`
	Layer        string         `json:"layer"` // 例如: Strategic, Tactical, Operational, Skill, Gateway, Adapter
	Action       string         `json:"action"`
	Status       string         `json:"status"` // 例如: success, failed, error, running
	StartedAt    time.Time      `json:"started_at"`
	EndedAt      time.Time      `json:"ended_at"`
	DurationMs   int64          `json:"duration_ms"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Span 代表链路追踪中的一个操作跨度，是追踪链路的组成单元。
// 它记录了特定层级（Layer）中的具体行为（Action）。
type Span struct {
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	OrgID        string         `json:"org_id,omitempty"`
	AgentID      string         `json:"agent_id,omitempty"`
	Layer        string         `json:"layer"`
	Action       string         `json:"action"`
	Status       string         `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	EndedAt      time.Time      `json:"ended_at,omitempty"`
	DurationMs   int64          `json:"duration_ms"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Trace 是由一组相关跨度构成的完整执行链路，描述了从 Strategic 到 Adapter 的完整拓扑。
type Trace struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id,omitempty"`
	Spans     []*Span   `json:"spans"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// TraceStorage 定义了追踪数据的持久化接口。
type TraceStorage interface {
	InsertTrace(t *Trace) error
	InsertSpan(s *Span) error
	UpdateSpan(s *Span) error
	GetTrace(traceID string) (*Trace, error)
	GetRecentTraces(orgID string, limit int) ([]*Trace, error)
}

// NewSpan 创建一个新的操作跨度实例。
func NewSpan(traceID, parentSpanID, action, layer string) *Span {
	return &Span{
		TraceID:      traceID,
		SpanID:       uuid.New().String(),
		ParentSpanID: parentSpanID,
		Action:       action,
		Layer:        layer,
		StartedAt:    time.Now(),
		Status:       "running",
		Metadata:     make(map[string]any),
	}
}

// NewRootTrace 初始化一个新的根追踪链路。
func NewRootTrace(orgID string) *Trace {
	return &Trace{
		ID:        uuid.New().String(),
		OrgID:     orgID,
		StartedAt: time.Now(),
		Spans:     make([]*Span, 0),
	}
}

// End 结束 Span 记录并自动计算持续时长（毫秒）。
func (s *Span) End(status string) {
	if s.EndedAt.IsZero() {
		s.EndedAt = time.Now()
		s.Status = status
		s.DurationMs = s.EndedAt.Sub(s.StartedAt).Milliseconds()
	}
}

// IsRoot 判断该 Span 是否为追踪链路的根起点。
func (s *Span) IsRoot() bool {
	return s.ParentSpanID == ""
}

// ToEvent 将当前 Span 的状态快照转换为 ExecutionEvent 格式。
func (s *Span) ToEvent() ExecutionEvent {
	return ExecutionEvent{
		TraceID:      s.TraceID,
		SpanID:       s.SpanID,
		ParentSpanID: s.ParentSpanID,
		OrgID:        s.OrgID,
		AgentID:      s.AgentID,
		Layer:        s.Layer,
		Action:       s.Action,
		Status:       s.Status,
		StartedAt:    s.StartedAt,
		EndedAt:      s.EndedAt,
		DurationMs:   s.DurationMs,
		Metadata:     s.Metadata,
	}
}

// AddSpan 将一个跨度关联到追踪链路中。
func (t *Trace) AddSpan(s *Span) {
	t.Spans = append(t.Spans, s)
	if s.EndedAt.After(t.EndedAt) {
		t.EndedAt = s.EndedAt
	}
}

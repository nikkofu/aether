package dag

import (
	"time"
)

// EventType 定义了流水线执行过程中可能发生的事件类型。
type EventType string

const (
	// EventNodeStarted 表示节点开始执行。
	EventNodeStarted EventType = "NODE_STARTED"
	// EventNodeCompleted 表示节点成功执行完成。
	EventNodeCompleted EventType = "NODE_COMPLETED"
	// EventNodeFailed 表示节点执行过程中发生错误。
	EventNodeFailed EventType = "NODE_FAILED"
	// EventPipelineStarted 表示整个流水线开始执行。
	EventPipelineStarted EventType = "PIPELINE_STARTED"
	// EventPipelineCompleted 表示整个流水线执行结束。
	EventPipelineCompleted EventType = "PIPELINE_COMPLETED"
)

// ExecutionEvent 包含了事件的详细上下文信息。
type ExecutionEvent struct {
	// Type 事件类型。
	Type EventType `json:"type"`
	// NodeID 触发事件的节点 ID（如果是流水线级别事件，该字段可能为空）。
	NodeID string `json:"node_id,omitempty"`
	// Timestamp 事件发生的时间。
	Timestamp time.Time `json:"timestamp"`
	// Duration 执行耗时（仅适用于 Completed/Failed 事件）。
	Duration time.Duration `json:"duration,omitempty"`
	// Error 执行错误（仅适用于 Failed 事件）。
	Error error `json:"error,omitempty"`
	// Metadata 可用于扩展其他自定义信息。
	Metadata map[string]any `json:"metadata,omitempty"`
}

// EventListener 是一个接口，允许外部组件订阅流水线的执行事件。
// 这为系统的可扩展性提供了基础，例如实现日志记录、指标收集 (Metrics) 或实时 UI 更新。
type EventListener interface {
	OnEvent(event ExecutionEvent)
}

// MultiEventListener 允许注册多个监听器。
type MultiEventListener []EventListener

// OnEvent 将事件分发给所有注册的监听器。
func (m MultiEventListener) OnEvent(event ExecutionEvent) {
	for _, l := range m {
		if l != nil {
			l.OnEvent(event)
		}
	}
}

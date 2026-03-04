package issue

import (
	"context"
	"time"
)

// Issue 代表系统检测到的一个异常、故障或风险事件。
type Issue struct {
	ID        string         `json:"id"`
	Severity  string         `json:"severity"`
	Source    string         `json:"source"` // 产生问题的组件或代理名
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Handler 定义了处理和上报 Issue 的标准接口。
// 通过实现此接口，可以轻松扩展 GitHub、GitLab 或 Sentry 等后端。
type Handler interface {
	// Report 上报一个 Issue。
	Report(ctx context.Context, iss Issue) error
	// Close 释放处理器资源。
	Close() error
}

// NewIssue 是一个便捷函数，用于创建一个带有当前时间戳的 Issue。
func NewIssue(id, severity, source, message string, metadata map[string]any) Issue {
	return Issue{
		ID:        id,
		Severity:  severity,
		Source:    source,
		Message:   message,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}
}

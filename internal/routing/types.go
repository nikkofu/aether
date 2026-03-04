package routing

import (
	"context"
)

// RequestMeta 包含了用于路由决策的所有元数据信息。
type RequestMeta struct {
	// Skill 是当前请求所调用的技能名称（如 "llm", "code-review"）。
	Skill string `json:"skill"`
	// PromptLength 是输入 Prompt 的大致字符数或 Token 数。
	PromptLength int `json:"prompt_length"`
	// RequiresFast 如果为 true，路由应优先选择低延迟的模型。
	RequiresFast bool `json:"requires_fast"`
	// RequiresCheap 如果为 true，路由应优先选择低成本的模型。
	RequiresCheap bool `json:"requires_cheap"`
	// Streaming 标识该请求是否需要流式输出支持。
	Streaming bool `json:"streaming"`
	// Metadata 允许扩展其他自定义路由属性。
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Router 定义了智能路由器的接口，负责为特定请求选择最佳的适配器。
type Router interface {
	// Select 根据请求元数据返回一个适配器的名称（如 "gemini", "openai"）。
	// 如果无法匹配到合适的适配器，应返回错误。
	Select(ctx context.Context, meta RequestMeta) (string, error)
}

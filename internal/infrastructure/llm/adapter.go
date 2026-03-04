package llm

import "context"

// TokenCallback 定义了处理流式输出片段的回调函数。
type TokenCallback func(token string)

// Adapter 定义了 Aether 与各种 CLI LLM 模型交互的标准接口。
type Adapter interface {
	// Name 返回此适配器的唯一标识名称（例如 "gemini", "claude"）。
	Name() string

	// Execute 发送 prompt 并阻塞直到获取完整的文本响应。
	// 适用于需要完整结果后再进行后续处理的场景。
	Execute(ctx context.Context, prompt string) (string, error)

	// Stream 发送 prompt 并以流式方式返回响应。
	// 每当适配器接收到新的文本片段（Token）时，都会调用 onToken 回调。
	// 这为终端交互提供了更好的用户体验。
	Stream(ctx context.Context, prompt string, onToken TokenCallback) error
}

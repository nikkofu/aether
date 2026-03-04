package evolution

import (
	"context"
	"time"
)

// StrategyTemplate 定义了 LLM 决策的核心 Prompt 模板及其版本元数据。
type StrategyTemplate struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Content   string    `json:"content"` // 具体的 Prompt 模板内容
	Score     float64   `json:"score"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// StrategyEngine 负责管理战略模板的注册、评估与自主进化。
type StrategyEngine interface {
	// Register 注册一个新的战略模板。
	Register(ctx context.Context, template StrategyTemplate) error
	// Activate 激活指定模板作为当前系统的核心决策逻辑。
	Activate(ctx context.Context, templateID string, version string) error
	// Evaluate 对战略模板执行 A/B 测试并返回评分。
	Evaluate(ctx context.Context, templateID string, version string) (float64, error)
	// Evolve 自动分析失败模式并尝试生成更优的战略版本。
	Evolve(ctx context.Context, orgID, templateID string) error
	// GetActive 获取当前生效的模板内容。
	GetActive(ctx context.Context, orgID string) (*StrategyTemplate, error)
}

package learning

import (
	"time"

	"github.com/nikkofu/aether/internal/reflection"
	"github.com/nikkofu/aether/internal/strategy"
)

// LearningEngine 负责基于反思结果更新代理策略。
type LearningEngine struct {
	strategyStore strategy.StrategyStore
}

// NewLearningEngine 创建一个新的学习引擎。
func NewLearningEngine(s strategy.StrategyStore) *LearningEngine {
	return &LearningEngine{strategyStore: s}
}

// UpdateStrategy 根据反思结果进化代理策略。
func (l *LearningEngine) UpdateStrategy(r *reflection.Reflection) error {
	s, err := l.strategyStore.Get(r.AgentName)
	if err != nil {
		// 如果不存在，初始化一个默认策略
		s = &strategy.Strategy{
			AgentName:  r.AgentName,
			RetryLimit: 3,
		}
	}

	// 1. 故障处理：增加重试上限
	if !r.Success {
		s.RetryLimit++
	}

	// 2. 成本优化：如果成本过高，提示使用更廉价的模型
	if r.Cost > 0.5 {
		s.RoutingHint = "cheap"
	}

	// 3. 性能优化：如果执行过慢，提示使用更快的模型
	if r.Duration > 3*time.Second {
		s.RoutingHint = "fast"
	}

	// 4. 学习总结：提取建议作为 Prompt 提示 (简单示例)
	if len(r.Suggestions) > 0 {
		s.PromptHint = r.Suggestions[0]
	}

	s.UpdatedAt = time.Now()
	return l.strategyStore.Save(s)
}

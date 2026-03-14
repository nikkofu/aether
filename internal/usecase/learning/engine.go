package learning

import (
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/domain/strategy"
	"github.com/nikkofu/aether/internal/usecase/reflection"
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
	if s.RetryLimit <= 0 {
		s.RetryLimit = 3
	}

	// 1. 故障处理：增加重试上限
	if !r.Success && s.RetryLimit < 6 {
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

	// 4. 学习总结：保留前两条高价值建议作为稳定提示
	for _, suggestion := range r.Suggestions {
		s.PromptHint = mergePromptHint(s.PromptHint, suggestion)
		if len(strings.Split(s.PromptHint, " | ")) >= 2 {
			break
		}
	}

	s.UpdatedAt = time.Now()
	return l.strategyStore.Save(s)
}

func mergePromptHint(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}

	parts := strings.Split(existing, " | ")
	parts = append(parts, next)
	parts = dedupeStrings(parts)
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	return strings.Join(parts, " | ")
}

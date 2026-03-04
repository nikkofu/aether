package evaluator

import (
	"context"
	"math/rand"
	"time"

	domain_skills "github.com/nikkofu/aether/internal/domain/capability/skills"
)

// SkillEvaluator 负责对技能及其演进版本进行质量评分。
type SkillEvaluator struct {
	engine domain_skills.SkillEngine
}

func NewSkillEvaluator(e domain_skills.SkillEngine) *SkillEvaluator {
	return &SkillEvaluator{engine: e}
}

// Evaluate 根据执行结果、延迟、成本等指标对 SkillVersion 进行评分。
func (e *SkillEvaluator) Evaluate(ctx context.Context, v domain_skills.SkillVersion, success bool, duration time.Duration) (float64, error) {
	// 简单的评分启发式：成功基础分，延迟惩罚
	score := 0.0
	if success {
		score = 0.8
		// 如果执行极快，则加分
		if duration < 500*time.Millisecond {
			score += 0.2
		}
	} else {
		score = 0.1
	}

	// 注入随机噪声（模拟环境不确定性）
	score += (rand.Float64() - 0.5) * 0.05

	if score > 1.0 { score = 1.0 }
	if score < 0.0 { score = 0.0 }

	return score, nil
}

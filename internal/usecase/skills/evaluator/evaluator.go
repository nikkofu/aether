package evaluator

import (
	"context"
	"time"

	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/internal/usecase/skills"
)

// SkillEvaluator 负责对技能版本进行自动化测试与质量评分。
type SkillEvaluator struct {
	engine skills.SkillEngine
	logger logging.Logger
}

func NewSkillEvaluator(e skills.SkillEngine, l logging.Logger) *SkillEvaluator {
	return &SkillEvaluator{engine: e, logger: l}
}

// EvaluateSkill 执行性能基准测试并更新技能版本的评分。
func (e *SkillEvaluator) EvaluateSkill(ctx context.Context, v *skills.SkillVersion) (float64, error) {
	// 1. 准备测试样本
	testSamples := []map[string]any{
		{"input_key": "sample1"},
		{"input_key": "sample2"},
	}

	var totalDuration time.Duration
	var successCount int

	// 2. 运行测试 (通常执行该具体版本的逻辑)
	for _, sample := range testSamples {
		start := time.Now()
		// 注意：这里的 Execute 逻辑需要能指定版本执行，或者我们暂时假设测试当前待评估的版本
		_, err := e.engine.Execute(ctx, v.SkillID, sample) 
		if err == nil {
			successCount++
			totalDuration += time.Since(start)
		}
	}

	// 3. 计算综合评分
	if len(testSamples) == 0 { return 0, nil }
	
	successRate := float64(successCount) / float64(len(testSamples))
	avgDuration := totalDuration.Seconds() / float64(len(testSamples))

	score := (successRate * 7.0)
	if avgDuration < 1.0 {
		score += 3.0
	} else if avgDuration < 3.0 {
		score += 1.5
	}

	// 4. 更新版本元数据
	v.Score = score
	// 覆盖更新版本信息（含评分）
	err := e.engine.RegisterVersion(ctx, *v)
	
	if e.logger != nil {
		e.logger.Info(ctx, "技能版本质量评估完成", 
			logging.String("skill_id", v.SkillID),
			logging.String("version", v.Version),
			logging.Float64("score", score))
	}

	return score, err
}

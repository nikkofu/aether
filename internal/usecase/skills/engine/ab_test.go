package engine

import (
	"context"
	"math/rand"
	"time"
)

// ABTestResult 记录单次测试的统计数据。
type ABTestResult struct {
	SuccessCount int
	TotalTime    time.Duration
	TotalCost    float64
}

// RunABTest 在两个技能版本间执行性能对比测试。
func (e *SkillEvolutionEngine) RunABTest(ctx context.Context, skillAID, skillBID string, sampleSize int) (string, error) {
	// 1. 准备测试样本 (实际应从 reflection.Store 获取)
	samples := []map[string]any{
		{"input": "test_1"}, {"input": "test_2"}, {"input": "test_3"},
	}

	statsA := &ABTestResult{}
	statsB := &ABTestResult{}

	// 2. 执行交叉对比测试
	for i := 0; i < sampleSize; i++ {
		sample := samples[rand.Intn(len(samples))]

		// 测试 A
		startA := time.Now()
		outA, errA := e.engine.Execute(ctx, skillAID, sample)
		if errA == nil {
			statsA.SuccessCount++
			statsA.TotalTime += time.Since(startA)
			cost, _ := outA["cost"].(float64)
			statsA.TotalCost += cost
		}

		// 测试 B
		startB := time.Now()
		outB, errB := e.engine.Execute(ctx, skillBID, sample)
		if errB == nil {
			statsB.SuccessCount++
			statsB.TotalTime += time.Since(startB)
			cost, _ := outB["cost"].(float64)
			statsB.TotalCost += cost
		}
	}

	// 3. 计算胜者 (成功率优先，其次耗时)
	rateA := float64(statsA.SuccessCount) / float64(sampleSize)
	rateB := float64(statsB.SuccessCount) / float64(sampleSize)

	if rateA > rateB { return skillAID, nil }
	if rateB > rateA { return skillBID, nil }

	// 成功率相同时，对比平均耗时
	if statsA.TotalTime < statsB.TotalTime { return skillAID, nil }
	return skillBID, nil
}

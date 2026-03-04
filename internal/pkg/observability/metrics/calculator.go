package metrics

import (
        "database/sql"
        "sort"
        "time"
)
type MetricsEngine struct {
	db *sql.DB
}

// NewMetricsEngine 创建一个新的指标引擎，利用 Trace 存储的底层 DB 进行查询
func NewMetricsEngine(db *sql.DB) *MetricsEngine {
	return &MetricsEngine{db: db}
}

type OrgMetrics struct {
	AverageTaskDurationMs float64            `json:"average_task_duration_ms"`
	P95TaskDurationMs     float64            `json:"p95_task_duration_ms"`
	P99TaskDurationMs     float64            `json:"p99_task_duration_ms"`
	AgentSuccessRates     map[string]float64 `json:"agent_success_rates"`
	SkillSuccessRates     map[string]float64 `json:"skill_success_rates"`
	CapabilityFrequencies map[string]int     `json:"capability_frequencies"`
}

// CalculateOrgMetrics 计算特定组织的运行指标
func (e *MetricsEngine) CalculateOrgMetrics(orgID string, since time.Time) (*OrgMetrics, error) {
	metrics := &OrgMetrics{
		AgentSuccessRates:     make(map[string]float64),
		SkillSuccessRates:     make(map[string]float64),
		CapabilityFrequencies: make(map[string]int),
	}

	// 计算任务(Span)耗时及其百分位
	// 这里提取所有的 Duration，用于计算 P95 和 P99
	rows, err := e.db.Query(`
		SELECT duration_ms FROM spans 
		WHERE org_id = ? AND duration_ms IS NOT NULL AND started_at >= ?
	`, orgID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var durations []float64
	var sum float64
	for rows.Next() {
		var d float64
		if err := rows.Scan(&d); err == nil {
			durations = append(durations, d)
			sum += d
		}
	}

	if len(durations) > 0 {
		metrics.AverageTaskDurationMs = sum / float64(len(durations))
		sort.Float64s(durations)
		
		p95Index := int(float64(len(durations)) * 0.95)
		if p95Index >= len(durations) { p95Index = len(durations) - 1 }
		metrics.P95TaskDurationMs = durations[p95Index]

		p99Index := int(float64(len(durations)) * 0.99)
		if p99Index >= len(durations) { p99Index = len(durations) - 1 }
		metrics.P99TaskDurationMs = durations[p99Index]
	}

	// 代理成功率 (Layer = Tactical or Operational)
	agentRows, err := e.db.Query(`
		SELECT agent_id, status, COUNT(*) 
		FROM spans 
		WHERE org_id = ? AND agent_id != '' AND started_at >= ?
		GROUP BY agent_id, status
	`, orgID, since)
	if err == nil {
		defer agentRows.Close()
		agentStats := make(map[string]struct{ success, total float64 })
		for agentRows.Next() {
			var agentID, status string
			var count float64
			if err := agentRows.Scan(&agentID, &status, &count); err == nil {
				stat := agentStats[agentID]
				stat.total += count
				if status == "success" {
					stat.success += count
				}
				agentStats[agentID] = stat
			}
		}
		for agentID, stat := range agentStats {
			if stat.total > 0 {
				metrics.AgentSuccessRates[agentID] = stat.success / stat.total
			}
		}
	}

	// 技能成功率 (Layer = Skill)
	skillRows, err := e.db.Query(`
		SELECT action, status, COUNT(*) 
		FROM spans 
		WHERE org_id = ? AND layer = 'Skill' AND started_at >= ?
		GROUP BY action, status
	`, orgID, since)
	if err == nil {
		defer skillRows.Close()
		skillStats := make(map[string]struct{ success, total float64 })
		for skillRows.Next() {
			var action, status string
			var count float64
			if err := skillRows.Scan(&action, &status, &count); err == nil {
				// action format is usually "wasm execution: {skillID}"
				stat := skillStats[action]
				stat.total += count
				if status == "success" {
					stat.success += count
				}
				skillStats[action] = stat
			}
		}
		for action, stat := range skillStats {
			if stat.total > 0 {
				metrics.SkillSuccessRates[action] = stat.success / stat.total
			}
		}
	}

	// 能力调用频率 (Layer = Gateway)
	capRows, err := e.db.Query(`
		SELECT action, COUNT(*) 
		FROM spans 
		WHERE org_id = ? AND layer = 'Gateway' AND started_at >= ?
		GROUP BY action
	`, orgID, since)
	if err == nil {
		defer capRows.Close()
		for capRows.Next() {
			var action string
			var count int
			if err := capRows.Scan(&action, &count); err == nil {
				metrics.CapabilityFrequencies[action] = count
			}
		}
	}

	return metrics, nil
}

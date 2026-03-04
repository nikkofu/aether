package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/pkg/logging"
)

// SentinelConfig 定义监控阈值。
type SentinelConfig struct {
	MaxDurationThreshold time.Duration
	CostBudget           float64
}

// SentinelAgent 负责系统监控。
type SentinelAgent struct {
	BaseAgent
	cfg    SentinelConfig
	logger logging.Logger
}

func NewSentinelAgent(name string, cfg SentinelConfig, l logging.Logger) *SentinelAgent {
	return &SentinelAgent{
		BaseAgent: *NewBaseAgent(name, "sentinel"),
		cfg:       cfg,
		logger:    l,
	}
}

func (a *SentinelAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	a.SetStatus(StatusRunning)
	defer a.SetStatus(StatusIdle)

	var alerts []Message
	switch msg.Type {
	case "system.span_report":
		duration, _ := msg.Payload["duration"].(time.Duration)
		if duration > a.cfg.MaxDurationThreshold {
			alerts = append(alerts, a.createAlert("超时告警", fmt.Sprintf("节点 %v 耗时 %v", msg.Payload["node_id"], duration), "HIGH"))
		}
	case "system.metrics_report":
		cost, _ := msg.Payload["estimated_cost"].(float64)
		if cost > a.cfg.CostBudget {
			alerts = append(alerts, a.createAlert("成本超支", fmt.Sprintf("消耗 $%v", cost), "CRITICAL"))
		}
	}
	return alerts, nil
}

func (a *SentinelAgent) createAlert(title, message, severity string) Message {
	return Message{
		ID:        uuid.New().String(),
		From:      a.name,
		To:        "issue_handler",
		Type:      "system.alert",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"title":    title,
			"message":  message,
			"severity": severity,
		},
	}
}

var _ Agent = (*SentinelAgent)(nil)

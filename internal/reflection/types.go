package reflection

import (
	"context"
	"time"
)

// Reflection 存储了对单次代理执行的深度评估结果。
type Reflection struct {
	ID           string        `json:"id"`
	AgentName    string        `json:"agent_name"`
	TaskID       string        `json:"task_id"`
	Success      bool          `json:"success"`
	Duration     time.Duration `json:"duration"`
	Cost         float64       `json:"cost"`
	ErrorMessage    string        `json:"error_message,omitempty"`
	Analysis        string        `json:"analysis"`
	Suggestions     []string      `json:"suggestions"`
	ConfidenceScore float64       `json:"confidence_score"`
	CreatedAt       time.Time     `json:"created_at"`
}

// ReflectionInput 包含了进行反思所需的基础数据。
type ReflectionInput struct {
	AgentName string
	TaskID    string
	Output    string
	Error     error
	Duration  time.Duration
	Cost      float64
}

// Reflector 定义了反思引擎的接口。
type Reflector interface {
	// Reflect 基于执行输入生成反思评估。
	Reflect(ctx context.Context, input ReflectionInput) (*Reflection, error)
}

// Store 定义了反思记录的持久化接口。
type Store interface {
	Save(ctx context.Context, r *Reflection) error
	ListRecent(ctx context.Context, limit int) ([]Reflection, error)
}

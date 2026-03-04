package capabilities

import (
	"context"
	"time"
)

// CapabilityRequest 封装了调用底层能力所需的完整上下文信息。
// 它是实现多租户隔离、权限校验和审计追踪的基础。
type CapabilityRequest struct {
	OrgID       string         `json:"org_id"`
	SkillID     string         `json:"skill_id"`
	UserID      string         `json:"user_id"`
	Name        string         `json:"name"`
	Params      map[string]any `json:"params"`
	RequestedAt time.Time      `json:"requested_at"`
}

// CapabilityResponse 定义了能力执行的标准返回格式。
type CapabilityResponse struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// Capability 定义了 Aether 系统底层原子能力的执行标准。
// 所有具体的能力实现（如模型调用、搜索、执行等）都应遵循此接口。
type Capability interface {
	// Name 返回该能力的唯一标识符。
	Name() string
	// Execute 执行原子能力逻辑。
	Execute(ctx context.Context, req CapabilityRequest) (CapabilityResponse, error)
}

// Gateway 作为能力访问的统筹入口，负责实施 RBAC、审计和多租户策略。
type Gateway interface {
	// Register 注册一个可用的原子能力。
	Register(cap Capability)
	// Execute 经过合规校验后执行请求的能力。
	Execute(ctx context.Context, req CapabilityRequest) (CapabilityResponse, error)
}

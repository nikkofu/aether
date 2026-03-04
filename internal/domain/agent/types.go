package agent

import (
	"context"
	"time"
)

// 核心系统消息类型
const (
	TypeSystemAlert   = "system.alert"
	TypeSystemSpawn   = "system.spawn"
	TypeTaskTender    = "system.task_tender"    // 任务招标
	TypeBidSubmission = "system.bid_submission" // 提交竞标
	TypeTaskAward     = "system.task_award"     // 授予任务
)

// AgentRoleFactory 定义了如何动态创建一个特定角色的代理。
type AgentRoleFactory func(ctx context.Context, name string, payload map[string]any) (Agent, error)

// Status 定义了代理生命周期的状态机阶段。
type Status string

const (
	StatusIdle       Status = "Idle"
	StatusRunning    Status = "Running"
	StatusWaiting    Status = "Waiting"
	StatusFailed     Status = "Failed"
	StatusCompleted  Status = "Completed"
	StatusTerminated Status = "Terminated"
)

// Message 定义了代理间通信的载体。
type Message struct {
	ID        string            `json:"id,omitempty"`
	From      string            `json:"from"`
	To        string            `json:"to"`
	Type      string            `json:"type"`
	Header    map[string]string `json:"header,omitempty"` // 用于 OTel 链路追踪传播
	Payload   map[string]any    `json:"payload"`
	Timestamp time.Time         `json:"timestamp"`
}

// Agent 是 Aether 系统的核心自治单元接口。
type Agent interface {
	Name() string
	Role() string
	Status() Status
	Handle(ctx context.Context, msg Message) ([]Message, error)
	Spawn(ctx context.Context, role string, payload map[string]any) (string, error)
	Shutdown(ctx context.Context) error
	SetBus(b Bus)
	SetStatus(s Status)
	Metadata() map[string]any
}

// Bus 定义消息发布接口。
type Bus interface {
	Publish(ctx context.Context, msg Message)
}

// AgentManager 定义了代理群的控制平面。
type AgentManager interface {
	Register(a Agent)
	RegisterRole(role string, factory AgentRoleFactory)
	Spawn(ctx context.Context, role string, payload map[string]any) (Agent, error)
	List() []Agent
	Get(name string) (Agent, bool)
	GetStats() ManagerStats
}

// ManagerStats 汇总了管理器的核心运行指标。
type ManagerStats struct {
	ActiveAgents  int            `json:"active_agents"`
	TotalSpawns   int64          `json:"total_spawns"`
	TotalFailures int64          `json:"total_failures"`
	StatusCounts  map[Status]int `json:"status_counts"`
}

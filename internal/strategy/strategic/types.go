package strategic

import (
	"context"
	"time"
)

// Vision 代表系统的长期宏观愿景。
type Vision struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// Goal 是为了实现愿景而设定的阶段性战略目标。
type Goal struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	VisionID    string    `json:"vision_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // planned | active | completed
	CreatedAt   time.Time `json:"created_at"`
}

// Milestone 是目标的最小可交付单元。
type Milestone struct {
	ID        string    `json:"id"`
	GoalID    string    `json:"goal_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"` // pending | active | done
	CreatedAt time.Time `json:"created_at"`
}

// StrategicPlan 汇总了完整的战略蓝图。
type StrategicPlan struct {
	Vision     Vision      `json:"vision"`
	Goals      []Goal      `json:"goals"`
	Milestones []Milestone `json:"milestones"`
}

// StrategicPlanner 定义了代理进行高层战略规划的能力接口。
type StrategicPlanner interface {
	// CreateVision 确立一个宏观愿景。
	CreateVision(ctx context.Context, title, desc string) (*Vision, error)
	// PlanGoals 将愿景分解为具体的战略目标。
	PlanGoals(ctx context.Context, vision Vision) ([]Goal, error)
	// PlanMilestones 将目标细化为可执行的里程碑。
	PlanMilestones(ctx context.Context, goal Goal) ([]Milestone, error)
	// Replan 基于执行反馈动态调整目标的执行路径。
	Replan(ctx context.Context, goal Goal, feedback string) ([]Milestone, error)
}

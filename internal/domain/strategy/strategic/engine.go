package strategic

import (
	"context"
	"fmt"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/pkg/observability/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Engine 负责驱动战略规划的执行循环。
type Engine struct {
	planner      StrategicPlanner
	store        Store
	agentManager agent.AgentManager
	bus          agent.Bus
	logger       logging.Logger
	tracer       *trace.TraceEngine
}

// NewEngine 创建一个新的战略执行引擎。
func NewEngine(p StrategicPlanner, s Store, am agent.AgentManager, b agent.Bus, l logging.Logger, t *trace.TraceEngine) *Engine {
	return &Engine{
		planner:      p,
		store:        s,
		agentManager: am,
		bus:          b,
		logger:       l,
		tracer:       t,
	}
}

// Start 启动战略执行循环。
func (e *Engine) Start(ctx context.Context) {
	e.logger.Info(ctx, "战略执行引擎已启动")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runCycle(ctx)
		}
	}
}

func (e *Engine) runCycle(ctx context.Context) {
	// 1. 加载活跃目标
	goals, err := e.store.ListActiveGoals()
	if err != nil {
		e.logger.Error(ctx, "加载活跃目标失败", logging.Err(err))
		return
	}

	for _, goal := range goals {
		e.processGoal(ctx, goal)
	}
}

func (e *Engine) processGoal(ctx context.Context, goal Goal) {
	// 2. 查找里程碑
	milestones, err := e.store.GetMilestones(goal.ID)
	if err != nil {
		return
	}

	// 如果没有里程碑，尝试规划
	if len(milestones) == 0 {
		e.logger.Info(ctx, "目标缺少里程碑，启动规划", logging.String("goal", goal.Title))
		
		// Tracing: strategic plan
		tracer := otel.Tracer("aether-tracer")
		var span oteltrace.Span
		ctx, span = tracer.Start(ctx, "strategic.plan")
		span.SetAttributes(
			attribute.String("org_id", goal.OrgID),
			attribute.String("goal_id", goal.ID),
			attribute.String("goal.title", goal.Title),
		)
		defer span.End()

		ms, err := e.planner.PlanMilestones(ctx, goal)
		if err == nil {
			e.store.SaveMilestones(ms)
			milestones = ms
		}
	}

	// 3. 找到第一个待处理的里程碑并执行
	var activeCount int
	var pending *Milestone
	for i := range milestones {
		if milestones[i].Status == "done" {
			continue
		}
		if milestones[i].Status == "active" {
			activeCount++
			continue
		}
		if pending == nil && milestones[i].Status == "pending" {
			pending = &milestones[i]
		}
	}

	// 如果当前有正在进行的里程碑，或者没有可进行的里程碑，则跳过
	if activeCount > 0 || pending == nil {
		// 检查是否所有里程碑已完成
		if activeCount == 0 && pending == nil && len(milestones) > 0 {
			e.store.UpdateGoalStatus(goal.ID, "completed")
			e.logger.Info(ctx, "战略目标已达成", logging.String("goal", goal.Title))
		}
		return
	}

	// 4. 转换为任务并下发给 Agent
	e.executeMilestone(ctx, goal, pending)
}

func (e *Engine) executeMilestone(ctx context.Context, goal Goal, ms *Milestone) {
	e.logger.Info(ctx, "启动里程碑任务", logging.String("milestone", ms.Title))
	
	// 更新状态为执行中
	e.store.UpdateMilestoneStatus(ms.ID, "active")

	// 构造任务消息发给总线
	msg := agent.Message{
		From:      "strategic_engine",
		To:        "supervisor",
		Type:      "task",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"description": fmt.Sprintf("战略目标 [%s] 的子任务: %s", goal.Title, ms.Title),
			"goal_id":     goal.ID,
			"ms_id":       ms.ID,
		},
	}

	if e.bus != nil {
		e.bus.Publish(ctx, msg)
	}
}

// HandleResult 应该由系统外部或消息监听器调用，用于更新里程碑结果。
// 此处演示逻辑：根据反馈决定是否 Replan。
func (e *Engine) HandleResult(ctx context.Context, goalID, msID string, success bool, feedback string) {
	if success {
		e.store.UpdateMilestoneStatus(msID, "done")
	} else {
		// 失败逻辑：如果失败严重，调用 Replan
		e.logger.Warn(ctx, "里程碑执行失败，触发重新规划", logging.String("goal_id", goalID))
		goal := Goal{ID: goalID} // 这里需要从 store 加载完整目标，简化处理
		newMilestones, err := e.planner.Replan(ctx, goal, feedback)
		if err == nil {
			// 这里实际生产中应该先清理旧的里程碑
			e.store.SaveMilestones(newMilestones)
		}
	}
}

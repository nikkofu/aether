package strategic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/core/capability"
	"github.com/nikkofu/aether/internal/core/knowledge"
	"github.com/nikkofu/aether/internal/core/strategy/evolution"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// LLMStrategicPlanner 实现了基于动态模板和知识增强的战略规划。
type LLMStrategicPlanner struct {
	llm            capability.Capability
	graph          knowledge.Graph
	strategyEngine evolution.StrategyEngine // 注入策略进化引擎
}

func NewLLMStrategicPlanner(llm capability.Capability, g knowledge.Graph, se evolution.StrategyEngine) *LLMStrategicPlanner {
	return &LLMStrategicPlanner{llm: llm, graph: g, strategyEngine: se}
}

func (p *LLMStrategicPlanner) CreateVision(ctx context.Context, title, desc string) (*Vision, error) {
	v := &Vision{ID: uuid.New().String(), Title: title, Description: desc, CreatedAt: time.Now()}
	if p.graph != nil {
		p.graph.AddEntity(ctx, knowledge.Entity{
			ID: v.ID, Type: "vision", Name: v.Title,
			Metadata: map[string]any{"description": v.Description},
		}, "default")
	}
	return v, nil
}

func (p *LLMStrategicPlanner) PlanGoals(ctx context.Context, v Vision) ([]Goal, error) {
	// Tracing: strategic plan
	tracer := otel.Tracer("aether-tracer")
	var span oteltrace.Span
	ctx, span = tracer.Start(ctx, "strategic.plan.goals")
	span.SetAttributes(
		attribute.String("vision_id", v.ID),
		attribute.String("vision_title", v.Title),
	)
	defer span.End()

	orgID := "default" // 简化演示
	
	// 1. 获取当前活跃的策略模板 (不再硬编码 Prompt)
	template, err := p.strategyEngine.GetActive(ctx, orgID)
	if err != nil || template == nil {
		return nil, fmt.Errorf("未找到生效的战略模板: %w", err)
	}

	historyCtx := p.getHistoryContext(ctx, "goal")

	// 2. 渲染模板
	prompt := template.Content
	prompt = strings.ReplaceAll(prompt, "{{history}}", historyCtx)
	prompt = strings.ReplaceAll(prompt, "{{vision_title}}", v.Title)
	prompt = strings.ReplaceAll(prompt, "{{vision_desc}}", v.Description)

	content, err := p.callLLM(ctx, prompt)
	if err != nil { return nil, err }

	var raw []struct {
		Title string `json:"title"`
		Description string `json:"description"`
	}
	json.Unmarshal([]byte(content), &raw)

	goals := make([]Goal, 0, len(raw))
	for _, rg := range raw {
		g := Goal{ID: uuid.New().String(), VisionID: v.ID, Title: rg.Title, Description: rg.Description, Status: "planned", CreatedAt: time.Now()}
		goals = append(goals, g)
		if p.graph != nil {
			p.graph.AddEntity(ctx, knowledge.Entity{ID: g.ID, Type: "goal", Name: g.Title}, orgID)
			p.graph.AddRelation(ctx, knowledge.Relation{ID: uuid.New().String(), FromID: v.ID, ToID: g.ID, Type: "has_goal"}, orgID)
		}
	}
	return goals, nil
}

func (p *LLMStrategicPlanner) PlanMilestones(ctx context.Context, g Goal) ([]Milestone, error) {
	// Tracing: strategic plan
	tracer := otel.Tracer("aether-tracer")
	var span oteltrace.Span
	ctx, span = tracer.Start(ctx, "strategic.plan.milestones")
	span.SetAttributes(
		attribute.String("goal_id", g.ID),
		attribute.String("goal_title", g.Title),
	)
	defer span.End()

	orgID := "default"
	
	template, err := p.strategyEngine.GetActive(ctx, orgID)
	if err != nil || template == nil { return nil, fmt.Errorf("template not found") }

	historyCtx := p.getHistoryContext(ctx, "milestone")
	
	// 这里假设模板支持 Milestone 生成分支，或者定义了专用占位符
	prompt := template.Content 
	prompt = strings.ReplaceAll(prompt, "{{history}}", historyCtx)
	prompt = strings.ReplaceAll(prompt, "{{goal_title}}", g.Title)

	content, err := p.callLLM(ctx, prompt)
	if err != nil { return nil, err }

	var raw []struct { Title string `json:"title"` }
	json.Unmarshal([]byte(content), &raw)

	milestones := make([]Milestone, 0, len(raw))
	for _, rm := range raw {
		m := Milestone{ID: uuid.New().String(), GoalID: g.ID, Title: rm.Title, Status: "pending", CreatedAt: time.Now()}
		milestones = append(milestones, m)
		if p.graph != nil {
			p.graph.AddEntity(ctx, knowledge.Entity{ID: m.ID, Type: "milestone", Name: m.Title}, orgID)
			p.graph.AddRelation(ctx, knowledge.Relation{ID: uuid.New().String(), FromID: g.ID, ToID: m.ID, Type: "has_milestone"}, orgID)
		}
	}
	return milestones, nil
}

func (p *LLMStrategicPlanner) Replan(ctx context.Context, g Goal, feedback string) ([]Milestone, error) {
	return p.PlanMilestones(ctx, g)
}

func (p *LLMStrategicPlanner) getHistoryContext(ctx context.Context, entityType string) string {
	if p.graph == nil { return "" }
	var sb strings.Builder
	reflections, _ := p.graph.QueryByType(ctx, "default", "reflection")
	count := 0
	for _, ref := range reflections {
		if conf, ok := ref.Metadata["confidence"].(float64); ok && conf > 0.6 {
			sb.WriteString(fmt.Sprintf("- 经验记录: %s\n", ref.Name))
			count++
			if count >= 5 { break }
		}
	}
	return sb.String()
}

func (p *LLMStrategicPlanner) callLLM(ctx context.Context, prompt string) (string, error) {
	output, err := p.llm.Execute(ctx, map[string]any{"prompt": prompt})
	if err != nil { return "", err }
	c, _ := output["output"].(string)
	c = strings.TrimPrefix(c, "```json")
	c = strings.TrimSuffix(c, "```")
	return strings.TrimSpace(c), nil
}

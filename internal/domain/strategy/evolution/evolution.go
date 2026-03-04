package evolution

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/internal/domain/policy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// DefaultStrategyEngine 实现了自主演进的战略引擎，集成安全守卫。
type DefaultStrategyEngine struct {
	llm       capability.Capability
	graph     knowledge.Graph
	templates map[string]*StrategyTemplate
	logger    logging.Logger
	guard     *policy.EvolutionGuard // 更新引用
}

func NewDefaultStrategyEngine(llm capability.Capability, g knowledge.Graph, l logging.Logger, guard *policy.EvolutionGuard) *DefaultStrategyEngine {
	return &DefaultStrategyEngine{
		llm:       llm,
		graph:     g,
		templates: make(map[string]*StrategyTemplate),
		logger:    l,
		guard:     guard,
	}
}

func (e *DefaultStrategyEngine) Register(ctx context.Context, t StrategyTemplate) error {
	e.templates[t.ID+t.Version] = &t
	return nil
}

func (e *DefaultStrategyEngine) Activate(ctx context.Context, id, version string) error {
	for _, t := range e.templates {
		if t.ID == id { t.Active = (t.Version == version) }
	}
	return nil
}

func (e *DefaultStrategyEngine) GetActive(ctx context.Context, orgID string) (*StrategyTemplate, error) {
	for _, t := range e.templates {
		if t.Active { return t, nil }
	}
	return &StrategyTemplate{
		ID: "default", Content: "愿景: {{vision_title}}\n历史: {{history}}\n生成3个目标，输出JSON。", Active: true,
	}, nil
}

// Evolve 自动分析失败模式并生成优化版 Prompt，受守卫管控。
func (e *DefaultStrategyEngine) Evolve(ctx context.Context, orgID, templateID string) error {
	// Tracing: strategic evolution
	tracer := otel.Tracer("aether-tracer")
	var span oteltrace.Span
	ctx, span = tracer.Start(ctx, "strategic.evolve")
	span.SetAttributes(
		attribute.String("org_id", orgID),
		attribute.String("template_id", templateID),
	)
	defer span.End()

	// 1. 守卫校验
	if e.guard != nil && !e.guard.AllowEvolution("strategy") {
		return fmt.Errorf("系统策略禁止战略进化")
	}

	// 2. 获取失败模式上下文
	reflections, _ := e.graph.QueryByType(ctx, orgID, "reflection")
	var failures []string
	for _, r := range reflections {
		if success, ok := r.Metadata["success"].(bool); ok && !success {
			failures = append(failures, r.Name)
			if len(failures) >= 5 { break }
		}
	}

	current, _ := e.GetActive(ctx, orgID)

	// 3. 调用 LLM 生成优化模板
	prompt := fmt.Sprintf(`你是战略优化专家。
当前模板: %s
最近失败模式: %v

请分析失败原因，并输出一个改进后的战略规划 Prompt 模板。
要求：
1. 包含 {{history}}, {{vision_title}}, {{vision_desc}} 占位符。
2. 提高对失败模式的防御性。
3. 仅输出模板内容：`, current.Content, failures)

	output, err := e.llm.Execute(ctx, map[string]any{"prompt": prompt})
	if err != nil { return err }
	newContent, _ := output["output"].(string)

	// 4. 注册新版本
	newV := StrategyTemplate{
		ID: templateID, Version: "v" + uuid.New().String()[:4],
		Content: newContent, CreatedAt: time.Now(),
	}
	e.Register(ctx, newV)

	// 5. 激活
	e.Activate(ctx, templateID, newV.Version)
	
	if e.logger != nil {
		e.logger.Info(ctx, "战略进化完成：已热激活优化版决策模板", 
			logging.String("org", orgID),
			logging.String("version", newV.Version))
	}

	return nil
}

func (e *DefaultStrategyEngine) Evaluate(ctx context.Context, id, version string) (float64, error) {
	return 0.8, nil
}

var _ StrategyEngine = (*DefaultStrategyEngine)(nil)

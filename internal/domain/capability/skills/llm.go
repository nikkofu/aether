package skills

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/infrastructure/llm"
	"github.com/nikkofu/aether/internal/infrastructure/llm/openai"
	"github.com/nikkofu/aether/pkg/metrics"
	"github.com/nikkofu/aether/pkg/observability"
	"github.com/nikkofu/aether/pkg/routing"
	"github.com/nikkofu/aether/internal/domain/strategy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type AdapterProvider interface {
	GetAdapter(name string) (cli_adapters.Adapter, bool)
}

// LLMSkill 是一个完全闭环的自进化能力实现。
type LLMSkill struct {
	name            string
	defaultAdapter  cli_adapters.Adapter
	adapterProvider AdapterProvider
	router          routing.Router
	tracker         metrics.Tracker
	tracer          observability.Tracer
	strategyStore   strategy.StrategyStore
	renderer        capability.PromptRenderer
	defaultTemplate string
}

func NewLLMSkill(name string, defaultAdapter cli_adapters.Adapter, provider AdapterProvider, router routing.Router, tracker metrics.Tracker, tracer observability.Tracer, strategyStore strategy.StrategyStore, renderer capability.PromptRenderer, template string) *LLMSkill {
	if renderer == nil {
		renderer = capability.NewDefaultRenderer()
	}
	return &LLMSkill{
		name:            name,
		defaultAdapter:  defaultAdapter,
		adapterProvider: provider,
		router:          router,
		tracker:         tracker,
		tracer:          tracer,
		strategyStore:   strategyStore,
		renderer:        renderer,
		defaultTemplate: template,
	}
}

func (s *LLMSkill) Name() string { return s.name }

func (s *LLMSkill) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Tracing: LLM Skill Execution using OTel
	tracer := otel.Tracer("aether-tracer")
	var span oteltrace.Span
	ctx, span = tracer.Start(ctx, "LLMSkill.Execute")
	span.SetAttributes(attribute.String("skill.name", s.name))
	defer span.End()

	if s.tracer != nil {
		var cSpan observability.Span
		_, cSpan = s.tracer.StartSpan(ctx, "LLMSkill.Execute", map[string]any{"skill": s.name})
		defer cSpan.End()
	}

	// 1. 加载策略优化参数
	agentName, _ := input["agent_name"].(string)
	var st *strategy.Strategy
	if s.strategyStore != nil {
		st, _ = s.strategyStore.Get(agentName)
	}
	if st == nil {
		st = &strategy.Strategy{RetryLimit: 1} // 默认不重试
	}

	// 2. 渲染并增强 Prompt (注入 PromptHint)
	promptTmpl := s.defaultTemplate
	if p, ok := input["prompt"].(string); ok && p != "" {
		promptTmpl = p
	}
	if st.PromptHint != "" {
		promptTmpl = fmt.Sprintf("[HINT: %s]\n%s", st.PromptHint, promptTmpl)
	}

	renderCtx := make(map[string]any)
	for k, v := range input { renderCtx[k] = v }
	prompt, err := s.renderer.Render(s.name, promptTmpl, renderCtx)
	if err != nil {
		return nil, err
	}

	// 3. 执行逻辑 (带重试循环)
	var lastErr error
	for attempt := 0; attempt < st.RetryLimit; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second) // 简单退避
		}

		// 选择适配器 (注入 RoutingHint)
		activeAdapter := s.selectAdapter(ctx, prompt, input, st)
		if activeAdapter == nil {
			return nil, fmt.Errorf("no adapter available")
		}

		result, err := s.callAdapter(ctx, activeAdapter, prompt, input)
		if err == nil {
			result["retry_count"] = attempt
			span.SetStatus(codes.Ok, "success")
			return result, nil
		}
		lastErr = err
	}

	span.RecordError(lastErr)
	span.SetStatus(codes.Error, lastErr.Error())
	return nil, fmt.Errorf("达到最大重试限制 (%d): %w", st.RetryLimit, lastErr)
}

func (s *LLMSkill) selectAdapter(ctx context.Context, prompt string, input map[string]any, st *strategy.Strategy) cli_adapters.Adapter {
	if name, ok := input["adapter"].(string); ok {
		if a, ok := s.adapterProvider.GetAdapter(name); ok { return a }
	}
	if s.router != nil {
		meta := routing.RequestMeta{Skill: s.name, PromptLength: len(prompt)}
		if st.RoutingHint == "cheap" { meta.RequiresCheap = true }
		if st.RoutingHint == "fast" { meta.RequiresFast = true }
		
		name, _ := s.router.Select(ctx, meta)
		if a, ok := s.adapterProvider.GetAdapter(name); ok { return a }
	}
	return s.defaultAdapter
}

func (s *LLMSkill) callAdapter(ctx context.Context, adapter cli_adapters.Adapter, prompt string, input map[string]any) (map[string]any, error) {
	var finalOutput string
	var usage openai.Usage
	streamEnabled, _ := input["stream"].(bool)
	oa, isOpenAI := adapter.(*openai.OpenAIAdapter)

	startTime := time.Now()
	if streamEnabled {
		var sb strings.Builder
		var streamErr error
		if isOpenAI {
			streamErr = oa.StreamWithUsage(ctx, prompt, func(t string) { fmt.Print(t); os.Stdout.Sync(); sb.WriteString(t) }, func(u openai.Usage) { usage = u })
		} else {
			streamErr = adapter.Stream(ctx, prompt, func(t string) { fmt.Print(t); os.Stdout.Sync(); sb.WriteString(t) })
		}
		if streamErr != nil { return nil, streamErr }
		fmt.Println()
		finalOutput = sb.String()
	} else {
		if isOpenAI {
			content, u, err := oa.ExecuteWithUsage(ctx, prompt)
			if err != nil { return nil, err }
			finalOutput, usage = content, u
		} else {
			content, err := adapter.Execute(ctx, prompt)
			if err != nil { return nil, err }
			finalOutput = content
		}
	}
	duration := time.Since(startTime)

	// 记录指标并计算成本
	cost := 0.0
	if isOpenAI && s.tracker != nil {
		cost = s.calculateCost("gpt-4o", usage.PromptTokens, usage.CompletionTokens)
		s.tracker.RecordUsage(ctx, metrics.UsageRecord{
			Provider: "openai", Model: "gpt-4o", PromptTokens: usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens, EstimatedCost: cost, CreatedAt: time.Now(),
		})
	}

	return map[string]any{
		"output":   finalOutput,
		"status":   "success",
		"adapter":  adapter.Name(),
		"cost":     cost,
		"duration": duration,
	}, nil
}

func (s *LLMSkill) calculateCost(model string, p, c int) float64 {
	return (float64(p) * 0.001 / 1000) + (float64(c) * 0.002 / 1000)
}

var _ capability.Capability = (*LLMSkill)(nil)

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
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/internal/domain/strategy"
	"github.com/nikkofu/aether/internal/domain/agent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type AdapterProvider interface {
	GetAdapter(name string) (llm.Adapter, bool)
}

// LLMSkill 是一个完全闭环的自进化能力实现。
type LLMSkill struct {
	name            string
	defaultAdapter  llm.Adapter
	adapterProvider AdapterProvider
	router          routing.Router
	tracker         metrics.Tracker
	tracer          observability.Tracer
	strategyStore   strategy.StrategyStore
	renderer        capability.PromptRenderer
	defaultTemplate string
	bus             bus.Bus // 新增总线支持
}

func NewLLMSkill(name string, defaultAdapter llm.Adapter, provider AdapterProvider, router routing.Router, tracker metrics.Tracker, tracer observability.Tracer, strategyStore strategy.StrategyStore, renderer capability.PromptRenderer, template string, b bus.Bus) *LLMSkill {
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
		bus:             b,
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

func (s *LLMSkill) selectAdapter(ctx context.Context, prompt string, input map[string]any, st *strategy.Strategy) llm.Adapter {
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

func (s *LLMSkill) callAdapter(ctx context.Context, adapter llm.Adapter, prompt string, input map[string]any) (map[string]any, error) {
	// ... [保持 MOCK 调试逻辑]
	if os.Getenv("AETHER_MOCK_LLM") == "true" {
		// ...
		return map[string]any{ /* ... */ }, nil
	}

	var finalOutput string
	
	// 强制开启 Stream 模式以支持实时反馈
	useStream := true 
	
	agentName, _ := input["agent_name"].(string)
	if agentName == "" { agentName = "llm" }

	startTime := time.Now()
	if useStream {
		var sb strings.Builder
		
		onToken := func(t string) {
			sb.WriteString(t)
			
			// 关键修复：改为异步非阻塞模式，确保 Token 回显不拖慢 LLM 推理
			if s.bus != nil {
				go func(token string) {
					// 使用 background 传递以避免原 context 超时影响回显
					s.bus.Publish(context.Background(), agent.Message{
						ID: fmt.Sprintf("tk-%d", time.Now().UnixNano()),
						From: s.name, To: "cli-feedback",
						Type: "token", Timestamp: time.Now(),
						Payload: map[string]any{
							"token": token,
							"agent": agentName,
						},
					})
				}(t)
			}
		}

		var streamErr error
		oa, isOpenAI := adapter.(*openai.OpenAIAdapter)
		if isOpenAI {
			streamErr = oa.StreamWithUsage(ctx, prompt, onToken, func(u openai.Usage) {})
		} else {
			streamErr = adapter.Stream(ctx, prompt, onToken)
		}
		
		if streamErr != nil {
			return nil, streamErr
		}
		finalOutput = sb.String()
	} else {
		oa, isOpenAI := adapter.(*openai.OpenAIAdapter)
		if isOpenAI {
			content, _, err := oa.ExecuteWithUsage(ctx, prompt)
			if err != nil { return nil, err }
			finalOutput = content
		} else {
			content, err := adapter.Execute(ctx, prompt)
			if err != nil { return nil, err }
			finalOutput = content
		}
	}
	duration := time.Since(startTime)

	return map[string]any{
		"output":   finalOutput,
		"status":   "success",
		"adapter":  adapter.Name(),
		"cost":     0.0,
		"duration": duration,
	}, nil
}

func (s *LLMSkill) calculateCost(model string, p, c int) float64 {
	return (float64(p) * 0.001 / 1000) + (float64(c) * 0.002 / 1000)
}

var _ capability.Capability = (*LLMSkill)(nil)

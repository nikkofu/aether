package skills

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/strategy"
	"github.com/nikkofu/aether/internal/infrastructure/llm"
	"github.com/nikkofu/aether/internal/infrastructure/llm/openai"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/pkg/metrics"
	"github.com/nikkofu/aether/pkg/observability"
	"github.com/nikkofu/aether/pkg/routing"
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
		promptTmpl = fmt.Sprintf("%s\n%s", renderLearnedRules(st.PromptHint), promptTmpl)
	}

	renderCtx := make(map[string]any)
	for k, v := range input {
		renderCtx[k] = v
	}
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
		if a, ok := s.adapterProvider.GetAdapter(name); ok {
			return a
		}
	}
	if s.router != nil {
		meta := routing.RequestMeta{Skill: s.name, PromptLength: len(prompt)}
		if st.RoutingHint == "cheap" {
			meta.RequiresCheap = true
		}
		if st.RoutingHint == "fast" {
			meta.RequiresFast = true
		}

		name, _ := s.router.Select(ctx, meta)
		if a, ok := s.adapterProvider.GetAdapter(name); ok {
			return a
		}
	}
	return s.defaultAdapter
}

func (s *LLMSkill) callAdapter(ctx context.Context, adapter llm.Adapter, prompt string, input map[string]any) (map[string]any, error) {
	if os.Getenv("AETHER_MOCK_LLM") == "true" {
		return mockLLMResponse(prompt, input), nil
	}

	var finalOutput string

	// 强制开启 Stream 模式以支持实时反馈
	useStream := true

	agentName, _ := input["agent_name"].(string)
	if agentName == "" {
		agentName = "llm"
	}
	taskID, _ := input["task_id"].(string)

	startTime := time.Now()
	if useStream {
		var sb strings.Builder

		onToken := func(t string) {
			if ctx.Err() != nil {
				return
			}
			sb.WriteString(t)

			// 关键修复：将 Token 发布到统一的 "cli" 主题，确保与 main.go 对齐
			if s.bus != nil {
				go func(token string) {
					payload := map[string]any{
						"token": token,
						"agent": agentName,
					}
					if taskID != "" {
						payload["task_id"] = taskID
					}
					s.bus.Publish(ctx, agent.Message{
						ID:   fmt.Sprintf("tk-%d", time.Now().UnixNano()),
						From: s.name, To: "cli",
						Type: "token", Timestamp: time.Now(),
						Payload: payload,
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
			if err != nil {
				return nil, err
			}
			finalOutput = content
		} else {
			content, err := adapter.Execute(ctx, prompt)
			if err != nil {
				return nil, err
			}
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

func renderLearnedRules(hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}

	parts := strings.Split(hint, " | ")
	var builder strings.Builder
	builder.WriteString("Learned operating rules from previous task reflections:\n")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(trimmed)
		builder.WriteString("\n")
	}
	builder.WriteString("Apply these learned rules unless the current task explicitly conflicts with them.")
	return builder.String()
}

func mockLLMResponse(prompt string, input map[string]any) map[string]any {
	agentName, _ := input["agent_name"].(string)
	normalizedAgent := strings.ToLower(agentName)
	normalizedPrompt := strings.ToLower(prompt)

	output := "Mock output generated by AETHER_MOCK_LLM."

	switch {
	case (strings.Contains(normalizedAgent, "tactical_manager") || strings.Contains(normalizedAgent, "tactical-manager")) &&
		(strings.Contains(normalizedPrompt, "请整合以下子任务输出") || strings.Contains(normalizedPrompt, "生成一个连贯的里程碑交付结果")):
		output = strings.Join([]string{
			"Coordinated milestone delivery:",
			"- Consolidated the worker outputs into one milestone-level result.",
			"- Preserved the key implementation decisions and handoff details.",
		}, "\n")
	case strings.Contains(normalizedAgent, "tactical_manager") || strings.Contains(normalizedAgent, "tactical-manager") || strings.Contains(normalizedPrompt, "拆解为 2-3 个具体的 go 开发任务"):
		output = strings.Join([]string{
			"[",
			"  \"Inspect the current workflow boundary and document the required coordination handoff.\",",
			"  \"Implement the worker-facing task execution path with explicit message routing.\",",
			"  \"Aggregate the worker outputs into a single coordination result for final delivery.\"",
			"]",
		}, "\n")
	case strings.Contains(normalizedAgent, "strategic_planner") || strings.Contains(normalizedPrompt, "可执行里程碑") || strings.Contains(normalizedPrompt, "战略目标"):
		if strings.Contains(normalizedPrompt, "里程碑") {
			output = strings.Join([]string{
				"[",
				"  {\"title\":\"Define the explicit strategic-to-tactical boundary.\"},",
				"  {\"title\":\"Execute the tactical decomposition and worker delivery path.\"}",
				"]",
			}, "\n")
		} else {
			output = strings.Join([]string{
				"[",
				"  {\"title\":\"Establish the workflow control plane\",\"description\":\"Standardize how tasks enter explicit workflow agents.\"},",
				"  {\"title\":\"Refactor orchestration boundaries\",\"description\":\"Move hidden coordination into explicit strategic and tactical agents.\"}",
				"]",
			}, "\n")
		}
	case strings.Contains(normalizedAgent, "reviewer") || strings.Contains(normalizedAgent, "reviewing") || strings.Contains(normalizedPrompt, "decision: [pass]") || strings.Contains(normalizedPrompt, "开始评审"):
		if parseMockIteration(input["iteration"]) == 1 {
			output = strings.Join([]string{
				"Thought: The first draft still needs one refinement cycle before approval.",
				"Decision: [FAIL]",
				"Feedback: Tighten the workflow handoff and make the review loop explicit.",
			}, "\n")
		} else {
			output = strings.Join([]string{
				"Thought: The implementation satisfies the mocked acceptance criteria and keeps the workflow moving.",
				"Decision: [PASS]",
				"Feedback: Mock review approved. No blocking issues detected.",
			}, "\n")
		}
	case strings.Contains(normalizedAgent, "planner") || strings.Contains(normalizedAgent, "reasoning") || strings.Contains(normalizedPrompt, "开始推理"):
		output = strings.Join([]string{
			"Thought: Identify the current workflow boundary, preserve the existing execution chain, and make the orchestration pattern explicit.",
			"Action: [{\"step\":\"Persist workflow_pattern on task creation and reads\"},{\"step\":\"Dispatch tasks through a workflow executor\"},{\"step\":\"Unify CLI and API entrypoints through TaskService\"}]",
			"Observation: The task can advance when planner, coder, and reviewer exchange explicit lifecycle messages.",
		}, "\n")
	default:
		output = strings.Join([]string{
			"package mock",
			"",
			"// Mock implementation produced by AETHER_MOCK_LLM.",
			"func Execute() string {",
			"\treturn \"mock result\"",
			"}",
		}, "\n")
	}

	return map[string]any{
		"output":   output,
		"status":   "success",
		"adapter":  "mock",
		"cost":     0.0,
		"duration": time.Millisecond,
	}
}

func parseMockIteration(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

var _ capability.Capability = (*LLMSkill)(nil)

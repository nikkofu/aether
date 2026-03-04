package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/pkg/observability"
)

// PlannerAgent 负责任务拆解并注入执行策略。
type PlannerAgent struct {
	BaseAgent
	llm     capability.Capability
	tracer  observability.Tracer
	manager AgentManager
	graph   knowledge.Graph
}

func NewPlannerAgent(name string, llm capability.Capability, tracer observability.Tracer) *PlannerAgent {
	return &PlannerAgent{
		BaseAgent: *NewBaseAgent(name, "planner"),
		llm:       llm,
		tracer:    tracer,
	}
}

func (a *PlannerAgent) SetManager(m AgentManager) { a.manager = m }
func (a *PlannerAgent) SetGraph(g knowledge.Graph) { a.graph = g }

func (a *PlannerAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	// 优先处理系统级消息（如竞标招标）
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if msg.Type != "task_plan_request" { return nil, nil }

		description, _ := msg.Payload["description"].(string)
		orgID, _ := msg.Payload["org_id"].(string)
		if orgID == "" { orgID = "default" }

		if a.tracer != nil {
			var span observability.Span
			ctx, span = a.tracer.StartSpan(ctx, "Planner.Handle", map[string]any{
				"msg_id":      msg.ID,
				"description": description,
			})
			defer span.End()

			// --- Lite RAG: 检索长程记忆 ---
			var memoryHints []string
			if a.graph != nil {
				searchKey := description
				if len(searchKey) > 20 { searchKey = searchKey[:20] }
				
				entities, _ := a.graph.Search(ctx, orgID, searchKey, 3)
				for _, e := range entities {
					if e.Type == "reflection" {
						analysis, _ := e.Metadata["analysis"].(string)
						if analysis != "" {
							memoryHints = append(memoryHints, analysis)
						}
					}
				}
				if len(memoryHints) > 0 {
					span.End() // 先结束之前的
					ctx, span = a.tracer.StartSpan(ctx, "Planner.MemoryRecall", map[string]any{
						"recalled_memories": memoryHints,
					})
				}
			}

			prompt := fmt.Sprintf("拆解任务：'%s'。按模块分工。", description)
			if len(memoryHints) > 0 {
				prompt = fmt.Sprintf("参考以下历史工程经验：\n%s\n\n基于以上经验，拆解当前任务：'%s'。请确保避免过去犯过的错误。", 
					strings.Join(memoryHints, "\n"), description)
			}

			// 记录 LLM 调用 Span
			llmCtx, llmSpan := a.tracer.StartSpan(ctx, "Planner.LLM_Inference", map[string]any{
				"final_prompt": prompt,
			})
			output, err := a.llm.Execute(llmCtx, map[string]any{"prompt": prompt, "agent_name": a.name})
			if err != nil {
				llmSpan.End()
				return nil, err
			}
			plan, _ := output["output"].(string)
			llmSpan.End()

			// 记录最终计划
			_, finalSpan := a.tracer.StartSpan(ctx, "Planner.Result", map[string]any{
				"generated_plan": plan,
			})
			finalSpan.End()

			if a.manager != nil {
				a.manager.Spawn(ctx, "coder", map[string]any{"task_id": msg.ID, "plan": plan})
			}

			return []Message{{
				From:      a.name,
				To:        "coder",
				Type:      "instruction",
				Timestamp: time.Now(),
				Payload: map[string]any{
					"plan": plan,
					"task": description,
				},
			}}, nil
		}
		return nil, nil // 兜底
	})
}

var _ Agent = (*PlannerAgent)(nil)

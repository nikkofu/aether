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
	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		if a.tracer != nil {
			var span observability.Span
			ctx, span = a.tracer.StartSpan(ctx, "Planner.Handle", map[string]any{"type": msg.Type})
			defer span.End()
		}

		if msg.Type != "task_plan_request" { return nil, nil }

		description, _ := msg.Payload["description"].(string)
		orgID, _ := msg.Payload["org_id"].(string)
		if orgID == "" { orgID = "default" }

		// --- Lite RAG: 检索长程记忆 ---
		var memoryHints []string
		if a.graph != nil {
			// 基于描述中的前 20 个字符进行简单的相似度检索
			searchKey := description
			if len(searchKey) > 20 { searchKey = searchKey[:20] }
			
			entities, _ := a.graph.Search(ctx, orgID, searchKey, 3)
			for _, e := range entities {
				if e.Type == "reflection" {
					analysis, _ := e.Metadata["analysis"].(string)
					if analysis != "" {
						memoryHints = append(memoryHints, fmt.Sprintf("- 历史经验(%s): %s", e.Name, analysis))
					}
				}
			}
		}

		prompt := fmt.Sprintf("拆解任务：'%s'。按模块分工。", description)
		if len(memoryHints) > 0 {
			prompt = fmt.Sprintf("参考以下历史工程经验：\n%s\n\n基于以上经验，拆解当前任务：'%s'。请确保避免过去犯过的错误。", 
				strings.Join(memoryHints, "\n"), description)
		}

		// 注入 agent_name 以便 Skill 获取策略
		input := map[string]any{
			"prompt":     prompt,
			"agent_name": a.name,
		}
		
		output, err := a.llm.Execute(ctx, input)
		if err != nil { return nil, err }
		
		plan, _ := output["output"].(string)

		if a.manager != nil {
			_, _ = a.manager.Spawn(ctx, "coder", map[string]any{"task_id": msg.ID})
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
	})
}

var _ Agent = (*PlannerAgent)(nil)

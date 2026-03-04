package org

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/agent"
	"github.com/nikkofu/aether/internal/capability"
	"github.com/nikkofu/aether/internal/economy"
	"github.com/nikkofu/aether/internal/observability/trace"
	"github.com/nikkofu/aether/internal/reflection"
	"github.com/nikkofu/aether/internal/skills/sandbox"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// OperationalWorkerAgent 集成了多租户经济激励与 WASM 技能执行。
type OperationalWorkerAgent struct {
	agent.BaseAgent
	supervisor   string
	llm          capability.Capability
	reflector    reflection.Reflector
	ledger       economy.Ledger
	wasmExecutor *sandbox.WASMExecutor // 注入 WASM 执行器
	traceEngine  *trace.TraceEngine
}

func NewOperationalWorkerAgent(id string, supervisor string, llm capability.Capability, ref reflection.Reflector, ledger economy.Ledger, wasm *sandbox.WASMExecutor, te *trace.TraceEngine) *OperationalWorkerAgent {
	return &OperationalWorkerAgent{
		BaseAgent:    *agent.NewBaseAgent(id, "operational"),
		supervisor:   supervisor,
		llm:          llm,
		reflector:    ref,
		ledger:       ledger,
		wasmExecutor: wasm,
		traceEngine:  te,
	}
}

func (a *OperationalWorkerAgent) ID() string           { return a.Name() }
func (a *OperationalWorkerAgent) Level() OrgLevel      { return LevelOperational }
func (a *OperationalWorkerAgent) Supervisor() string    { return a.supervisor }
func (a *OperationalWorkerAgent) Subordinates() []string { return nil }

func (a *OperationalWorkerAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		if msg.Type != "task.assigned" { return nil, nil }

		orgID, _ := msg.Payload["org_id"].(string)
		if orgID == "" { orgID = "default" }

		// Tracing: skill execution using OTel via traceEngine
		if a.traceEngine != nil {
			var span oteltrace.Span
			ctx, span = a.traceEngine.StartSpan(ctx, "skill execution")
			span.SetAttributes(
				attribute.String("org_id", orgID),
				attribute.String("skill_id", fmt.Sprintf("%v", msg.Payload["skill_id"])),
			)
			defer span.End()
		}

		skillID, _ := msg.Payload["skill_id"].(string)
		wasmPath, _ := msg.Payload["wasm_path"].(string)

		var output map[string]any
		var err error
		start := time.Now()

		// 如果指定了 WASM 技能，则使用沙箱执行
		if skillID != "" && a.wasmExecutor != nil {
			inputBytes, _ := msg.Payload["input"].([]byte)
			resBytes, execErr := a.wasmExecutor.Execute(ctx, orgID, a.Name(), skillID, wasmPath, inputBytes)
			err = execErr
			output = map[string]any{"output": string(resBytes), "cost": 0.01} // 沙箱固定小额费用
		} else {
			// 否则回退到 LLM 逻辑
			tasks, _ := msg.Payload["tasks"].(string)
			input := map[string]any{"prompt": tasks, "agent_name": a.Name(), "org_id": orgID}
			output, err = a.llm.Execute(ctx, input)
		}

		duration := time.Since(start)

		// 经济闭环
		if a.ledger != nil {
			cost, _ := output["cost"].(float64)
			_ = a.ledger.UpdateBalance(ctx, orgID, a.Name(), -cost, 0)
			_ = a.ledger.AddTransaction(ctx, economy.Transaction{
				ID: uuid.New().String(), OrgID: orgID, From: a.Name(), To: "system", Amount: cost, Type: "cost",
			})

			if err == nil {
				_ = a.ledger.UpdateBalance(ctx, orgID, a.Name(), cost*0.2, 1.0)
			} else {
				_ = a.ledger.UpdateBalance(ctx, orgID, a.Name(), 0, -1.0)
			}
		}

		refInput := reflection.ReflectionInput{
			AgentName: a.Name(), TaskID: msg.ID, Output: fmt.Sprintf("%v", output["output"]),
			Error: err, Duration: duration,
		}
		reflectResult, _ := a.reflector.Reflect(ctx, refInput)

		return []agent.Message{
			{
				From: a.Name(), To: a.supervisor, Type: "task.completed",
				Timestamp: time.Now(), Payload: map[string]any{
					"output": output["output"], "goal_id": msg.Payload["goal_id"], "ms_id": msg.Payload["ms_id"], "org_id": orgID,
				},
			},
			{
				From: a.Name(), To: "governance", Type: "reflection.report",
				Timestamp: time.Now(), Payload: map[string]any{"reflection": reflectResult, "org_id": orgID},
			},
		}, nil
	})
}

var _ OrgAgent = (*OperationalWorkerAgent)(nil)

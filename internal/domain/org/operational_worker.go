package org

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/economy"
	"github.com/nikkofu/aether/internal/usecase/reflection"
	"github.com/nikkofu/aether/internal/usecase/skills/sandbox"
	"github.com/nikkofu/aether/pkg/observability/trace"
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

func (a *OperationalWorkerAgent) ID() string             { return a.Name() }
func (a *OperationalWorkerAgent) Level() OrgLevel        { return LevelOperational }
func (a *OperationalWorkerAgent) Supervisor() string     { return a.supervisor }
func (a *OperationalWorkerAgent) Subordinates() []string { return nil }

func (a *OperationalWorkerAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		if msg.Type != "task.assigned" {
			return nil, nil
		}

		orgID, _ := msg.Payload["org_id"].(string)
		if orgID == "" {
			orgID = "default"
		}

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
			input := map[string]any{"prompt": buildOperationalTaskPrompt(tasks), "agent_name": a.Name(), "org_id": orgID}
			output, err = a.llm.Execute(ctx, input)
		}
		if output == nil {
			output = make(map[string]any)
		}

		duration := time.Since(start)

		// 经济闭环
		if !ledgerIsNil(a.ledger) {
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
		var reflectResult *reflection.Reflection
		if a.reflector != nil {
			reflectResult, _ = a.reflector.Reflect(ctx, refInput)
		}

		feedback := ""
		if err != nil {
			feedback = err.Error()
		}

		messages := []agent.Message{
			{
				ID:   msg.ID,
				From: a.Name(), To: a.supervisor, Type: "task.completed",
				Timestamp: time.Now(), Payload: map[string]any{
					"success":         err == nil,
					"output":          output["output"],
					"feedback":        feedback,
					"error":           feedback,
					"goal_id":         msg.Payload["goal_id"],
					"milestone_id":    msg.Payload["milestone_id"],
					"ms_id":           msg.Payload["ms_id"],
					"org_id":          orgID,
					"task_id":         msg.Payload["task_id"],
					"trace_id":        msg.Payload["trace_id"],
					"delivery_target": msg.Payload["delivery_target"],
					"delivery_type":   msg.Payload["delivery_type"],
					"branch_name":     msg.Payload["branch_name"],
					"subtask_index":   msg.Payload["subtask_index"],
					"subtask_total":   msg.Payload["subtask_total"],
				},
			},
		}

		if reflectResult != nil {
			messages = append(messages, agent.Message{
				ID:   msg.ID,
				From: a.Name(), To: "governance", Type: "reflection.report",
				Timestamp: time.Now(), Payload: map[string]any{"reflection": reflectResult, "org_id": orgID},
			})
		}

		return messages, nil
	})
}

func ledgerIsNil(ledger economy.Ledger) bool {
	if ledger == nil {
		return true
	}
	value := reflect.ValueOf(ledger)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func buildOperationalTaskPrompt(task string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		return ""
	}

	if containsHan(task) {
		return fmt.Sprintf(
			"你是执行型代理。请直接完成下面的子任务，并输出最终可交付结果。\n要求:\n1. 直接给结果，不要写“我将做什么”、前言或元说明。\n2. 严格遵守子任务中的格式、长度、JSON、清单或精确前缀要求。\n3. 如果任务要求列点、JSON、标题或步骤，只输出该最终结构。\n4. 如果上下文有限，只基于子任务给出的事实作答，不要编造外部事实。\n\n子任务:\n%s",
			task,
		)
	}

	return fmt.Sprintf(
		"You are an execution-focused worker. Complete the subtask and return only the final deliverable.\nRequirements:\n1. Return the final result directly with no preface, plan, or meta commentary.\n2. Follow any explicit format, length, JSON, checklist, or exact-prefix requirements literally.\n3. If the task asks for bullets, JSON, headings, or steps, output only that final structure.\n4. If context is limited, answer only from the facts in the subtask and do not invent external facts.\n\nSubtask:\n%s",
		task,
	)
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

var _ OrgAgent = (*OperationalWorkerAgent)(nil)

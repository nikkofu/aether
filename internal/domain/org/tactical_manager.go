package org

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/pkg/observability"
)

// TacticalManagerAgent 负责战术执行，将里程碑拆解为任务并指派工人。
type TacticalManagerAgent struct {
	agent.BaseAgent
	supervisor   string
	subs         []string
	agentManager agent.AgentManager
	llm          capability.Capability
	tracer       observability.Tracer
	mu           sync.Mutex
	states       map[string]*tacticalTaskState
}

type tacticalTaskState struct {
	CoordinationKey string
	TaskID          string
	GoalID          string
	MilestoneID     string
	MilestoneTitle  string
	TraceID         string
	DeliveryTarget  string
	DeliveryType    string
	Pending         int
	Outputs         []tacticalSubtaskOutput
	Errors          []string
}

type tacticalSubtaskOutput struct {
	Index   int
	Content string
}

func NewTacticalManagerAgent(id string, supervisor string, am agent.AgentManager, llm capability.Capability, t observability.Tracer) *TacticalManagerAgent {
	return &TacticalManagerAgent{
		BaseAgent:    *agent.NewBaseAgent(id, "tactical-manager"),
		supervisor:   supervisor,
		agentManager: am,
		llm:          llm,
		tracer:       t,
		states:       make(map[string]*tacticalTaskState),
	}
}

func (a *TacticalManagerAgent) ID() string             { return a.Name() }
func (a *TacticalManagerAgent) Level() OrgLevel        { return LevelTactical }
func (a *TacticalManagerAgent) Supervisor() string     { return a.supervisor }
func (a *TacticalManagerAgent) Subordinates() []string { return a.subs }

func (a *TacticalManagerAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	return a.ProtectedHandle(ctx, msg, func() ([]agent.Message, error) {
		if msg.Type == "milestone.assigned" {
			milestone, _ := msg.Payload["milestone"].(map[string]any)
			if milestone == nil {
				return nil, fmt.Errorf("tactical manager requires milestone payload")
			}
			if a.llm == nil {
				return nil, fmt.Errorf("tactical manager LLM is not initialized")
			}
			if a.agentManager == nil {
				return nil, fmt.Errorf("tactical manager agent manager is not initialized")
			}
			goalID, _ := msg.Payload["goal_id"].(string)
			milestoneID, _ := msg.Payload["milestone_id"].(string)
			if milestoneID == "" {
				if rawID, ok := milestone["id"].(string); ok {
					milestoneID = rawID
				}
			}
			milestoneTitle := firstNonEmptyString(milestone["title"], milestone["name"], milestoneID)
			deliveryTarget := a.supervisor
			if target, ok := msg.Payload["delivery_target"].(string); ok && target != "" {
				deliveryTarget = target
			}
			deliveryType := "milestone.feedback"
			if typed, ok := msg.Payload["delivery_type"].(string); ok && typed != "" {
				deliveryType = typed
			}

			// 1. 拆解里程碑为任务
			input := map[string]any{
				"prompt":     buildMilestoneBreakdownPrompt(milestoneTitle),
				"agent_name": a.Name(),
				"task_id":    msg.Payload["task_id"],
				"org_id":     msg.Payload["org_id"],
			}
			output, err := a.llm.Execute(ctx, input)
			if err != nil {
				return nil, err
			}
			if output == nil {
				return nil, fmt.Errorf("tactical manager LLM returned nil output")
			}

			tasks, ok := output["output"].(string)
			if !ok || tasks == "" {
				return nil, fmt.Errorf("tactical manager LLM did not return task breakdown")
			}

			subtasks := parseSubtasks(tasks)
			if len(subtasks) == 0 {
				return nil, fmt.Errorf("tactical manager failed to parse subtasks")
			}

			taskID := firstNonEmptyString(msg.Payload["task_id"], msg.ID)
			coordinationKey := tacticalStateKey(taskID, milestoneID)
			state := &tacticalTaskState{
				CoordinationKey: coordinationKey,
				TaskID:          taskID,
				GoalID:          goalID,
				MilestoneID:     milestoneID,
				MilestoneTitle:  milestoneTitle,
				TraceID:         firstNonEmptyString(msg.Payload["trace_id"]),
				DeliveryTarget:  deliveryTarget,
				DeliveryType:    deliveryType,
				Pending:         len(subtasks),
				Outputs:         make([]tacticalSubtaskOutput, 0, len(subtasks)),
				Errors:          make([]string, 0, len(subtasks)),
			}
			a.storeState(state)

			messages := make([]agent.Message, 0, len(subtasks))
			for index, subtask := range subtasks {
				workerName, err := a.agentManager.Spawn(ctx, "operational", map[string]any{
					"task_id": taskID,
					"org_id":  msg.Payload["org_id"],
				})
				if err != nil {
					a.deleteState(coordinationKey)
					return nil, err
				}

				messages = append(messages, agent.Message{
					ID:        msg.ID,
					From:      a.Name(),
					To:        workerName.Name(),
					Type:      "task.assigned",
					Timestamp: time.Now(),
					Payload: map[string]any{
						"tasks":           subtask,
						"goal_id":         goalID,
						"milestone_id":    milestoneID,
						"ms_id":           milestoneID,
						"org_id":          msg.Payload["org_id"],
						"task_id":         taskID,
						"trace_id":        msg.Payload["trace_id"],
						"delivery_target": a.Name(),
						"delivery_type":   "task.completed",
						"subtask_index":   index + 1,
						"subtask_total":   len(subtasks),
					},
				})
			}

			return messages, nil
		}

		if msg.Type == "task.completed" {
			taskID := firstNonEmptyString(msg.Payload["task_id"], msg.ID)
			coordinationKey := tacticalStateKey(taskID, firstNonEmptyString(msg.Payload["milestone_id"], msg.Payload["ms_id"]))
			state, ok := a.recordTaskCompletion(coordinationKey, msg)
			if !ok {
				return nil, fmt.Errorf("tactical manager state not found for task %s", coordinationKey)
			}
			if state.Pending > 0 {
				return nil, nil
			}

			success := len(state.Errors) == 0
			output := joinOrderedSubtaskOutputs(state.Outputs)
			if success {
				output = a.synthesizeMilestoneOutput(ctx, state, output)
			}
			feedback := strings.Join(state.Errors, "\n")
			a.deleteState(state.CoordinationKey)

			return []agent.Message{{
				ID:        msg.ID,
				From:      a.Name(),
				To:        state.DeliveryTarget,
				Type:      state.DeliveryType,
				Timestamp: time.Now(),
				Payload: map[string]any{
					"success":      success,
					"goal_id":      state.GoalID,
					"milestone_id": state.MilestoneID,
					"ms_id":        state.MilestoneID,
					"task_id":      state.TaskID,
					"trace_id":     state.TraceID,
					"output":       output,
					"feedback":     feedback,
					"error":        feedback,
				},
			}}, nil
		}

		return nil, nil
	})
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if str, ok := value.(string); ok && str != "" {
			return str
		}
	}
	return ""
}

func parseSubtasks(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var subtasks []string
	if err := json.Unmarshal([]byte(trimmed), &subtasks); err == nil {
		return cleanSubtasks(subtasks)
	}

	start := strings.Index(trimmed, "[")
	end := strings.LastIndex(trimmed, "]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(trimmed[start:end+1]), &subtasks); err == nil {
			return cleanSubtasks(subtasks)
		}
	}

	return cleanSubtasks([]string{trimmed})
}

func cleanSubtasks(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func (a *TacticalManagerAgent) storeState(state *tacticalTaskState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.states[state.CoordinationKey] = state
}

func (a *TacticalManagerAgent) recordTaskCompletion(coordinationKey string, msg agent.Message) (tacticalTaskState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[coordinationKey]
	if !ok || state == nil {
		return tacticalTaskState{}, false
	}

	if output, ok := msg.Payload["output"].(string); ok && strings.TrimSpace(output) != "" {
		subtaskIndex := parsePositiveIntValue(msg.Payload["subtask_index"], 0)
		if subtaskIndex <= 0 {
			subtaskIndex = len(state.Outputs) + 1
		}
		state.Outputs = append(state.Outputs, tacticalSubtaskOutput{
			Index:   subtaskIndex,
			Content: strings.TrimSpace(output),
		})
	}
	success, _ := msg.Payload["success"].(bool)
	if !success {
		failure := firstNonEmptyString(msg.Payload["feedback"], msg.Payload["error"])
		if failure == "" {
			failure = fmt.Sprintf("subtask failed for task %s", state.TaskID)
		}
		state.Errors = append(state.Errors, failure)
	}
	if state.Pending > 0 {
		state.Pending--
	}

	return *state, true
}

func (a *TacticalManagerAgent) synthesizeMilestoneOutput(ctx context.Context, state tacticalTaskState, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if len(state.Outputs) <= 1 || a.llm == nil {
		return fallback
	}

	prompt := fmt.Sprintf(
		"你是战术协调者。请整合以下子任务输出，生成一个连贯的里程碑交付结果。\n里程碑: %s\n要求:\n1. 保留关键结论、执行结果和风险。\n2. 去重并按逻辑顺序组织。\n3. 直接输出最终交付文本，不要解释过程。\n\n子任务输出:\n%s",
		state.MilestoneTitle,
		formatSubtaskOutputsForPrompt(state.Outputs),
	)
	result, err := a.llm.Execute(ctx, map[string]any{
		"prompt":     prompt,
		"agent_name": a.Name(),
		"task_id":    state.TaskID,
	})
	if err != nil || result == nil {
		return fallback
	}

	output, _ := result["output"].(string)
	output = strings.TrimSpace(output)
	if output == "" {
		return fallback
	}
	return output
}

func buildMilestoneBreakdownPrompt(milestoneTitle string) string {
	return fmt.Sprintf(
		"将里程碑 '%s' 拆解为 2-3 个可以直接执行的子任务。根据里程碑内容选择合适的任务类型，例如开发、验证、部署、观测、文档或交付。请只输出 JSON 字符串数组，不要附加解释。",
		milestoneTitle,
	)
}

func (a *TacticalManagerAgent) deleteState(coordinationKey string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.states, coordinationKey)
}

func tacticalStateKey(taskID, milestoneID string) string {
	if milestoneID == "" {
		return taskID
	}
	if taskID == "" {
		return milestoneID
	}
	return taskID + ":" + milestoneID
}

func joinOrderedSubtaskOutputs(outputs []tacticalSubtaskOutput) string {
	if len(outputs) == 0 {
		return ""
	}

	ordered := append([]tacticalSubtaskOutput(nil), outputs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Index < ordered[j].Index
	})

	parts := make([]string, 0, len(ordered))
	for _, output := range ordered {
		if strings.TrimSpace(output.Content) == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(output.Content))
	}

	return strings.Join(parts, "\n\n")
}

func formatSubtaskOutputsForPrompt(outputs []tacticalSubtaskOutput) string {
	if len(outputs) == 0 {
		return ""
	}

	ordered := append([]tacticalSubtaskOutput(nil), outputs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Index < ordered[j].Index
	})

	lines := make([]string, 0, len(ordered))
	for _, output := range ordered {
		if strings.TrimSpace(output.Content) == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s", output.Index, strings.TrimSpace(output.Content)))
	}

	return strings.Join(lines, "\n")
}

func parsePositiveIntValue(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int32:
		if typed > 0 {
			return int(typed)
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float32:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		if typed == "" {
			return fallback
		}
		var parsed int
		if _, err := fmt.Sscanf(typed, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

var _ OrgAgent = (*TacticalManagerAgent)(nil)

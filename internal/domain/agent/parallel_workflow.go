package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	taskdomain "github.com/nikkofu/aether/internal/domain/task"
)

const ParallelWorkflowAgentName = "workflow.parallel"

type parallelBranchSpec struct {
	Index int
	Name  string
	Task  string
}

type parallelBranchResult struct {
	Index  int
	Output string
}

type parallelWorkflowState struct {
	TaskID   string
	Task     string
	TraceID  string
	OrgID    string
	Branches []parallelBranchSpec
	Pending  int
	Results  []parallelBranchResult
	Errors   []string
}

type ParallelWorkflowAgent struct {
	BaseAgent
	agentManager AgentManager
	mu           sync.Mutex
	states       map[string]*parallelWorkflowState
}

func NewParallelWorkflowAgent(name string, manager AgentManager) *ParallelWorkflowAgent {
	if name == "" {
		name = ParallelWorkflowAgentName
	}

	return &ParallelWorkflowAgent{
		BaseAgent:    *NewBaseAgent(name, "workflow-parallel"),
		agentManager: manager,
		states:       make(map[string]*parallelWorkflowState),
	}
}

func (a *ParallelWorkflowAgent) Handle(ctx context.Context, msg Message) ([]Message, error) {
	if sysMsgs := a.HandleSystemMessage(ctx, msg); sysMsgs != nil {
		return sysMsgs, nil
	}

	return a.ProtectedHandle(ctx, msg, func() ([]Message, error) {
		switch msg.Type {
		case TypeWorkflowParallelStart:
			return a.handleStart(ctx, msg)
		case "task.completed":
			return a.handleTaskCompleted(msg)
		default:
			return nil, nil
		}
	})
}

func (a *ParallelWorkflowAgent) handleStart(ctx context.Context, msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	task, _ := msg.Payload["description"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("parallel workflow requires task_id")
	}
	if task == "" {
		return nil, fmt.Errorf("parallel workflow requires task description")
	}
	if a.agentManager == nil {
		return nil, fmt.Errorf("parallel workflow requires an agent manager")
	}

	branches := resolveParallelBranches(
		task,
		msg.Payload[taskdomain.ParallelBranchesInputKey],
		msg.Payload[taskdomain.LegacyParallelTasksInputKey],
	)
	if len(branches) == 0 {
		return nil, fmt.Errorf("parallel workflow requires at least one branch")
	}

	state := &parallelWorkflowState{
		TaskID:   taskID,
		Task:     task,
		TraceID:  stringValue(msg.Payload["trace_id"]),
		OrgID:    firstNonEmptyString(msg.Payload["org_id"], "default"),
		Branches: branches,
		Pending:  len(branches),
		Results:  make([]parallelBranchResult, 0, len(branches)),
		Errors:   make([]string, 0, len(branches)),
	}

	messages := make([]Message, 0, len(branches))
	for _, branch := range branches {
		worker, err := a.agentManager.Spawn(ctx, "operational", map[string]any{
			"task_id":    taskID,
			"org_id":     state.OrgID,
			"supervisor": a.name,
		})
		if err != nil {
			return []Message{a.failureAlert(msg.ID, taskID, state.TraceID, fmt.Sprintf("parallel workflow failed to spawn worker for branch %d: %v", branch.Index, err))}, nil
		}

		messages = append(messages, Message{
			ID:        msg.ID,
			From:      a.name,
			To:        worker.Name(),
			Type:      "task.assigned",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"tasks":         branch.Task,
				"task_id":       taskID,
				"trace_id":      state.TraceID,
				"org_id":        state.OrgID,
				"branch_name":   branch.Name,
				"subtask_index": branch.Index,
				"subtask_total": len(branches),
			},
		})
	}

	a.mu.Lock()
	a.states[taskID] = state
	a.mu.Unlock()

	return messages, nil
}

func (a *ParallelWorkflowAgent) handleTaskCompleted(msg Message) ([]Message, error) {
	taskID, _ := msg.Payload["task_id"].(string)
	state, ok := a.recordTaskCompletion(taskID, msg)
	if !ok {
		return nil, fmt.Errorf("parallel workflow state not found for task %s", taskID)
	}
	if state.Pending > 0 {
		return nil, nil
	}

	a.deleteState(taskID)

	if len(state.Errors) > 0 {
		return []Message{a.failureAlert(msg.ID, taskID, state.TraceID, strings.Join(state.Errors, "\n"))}, nil
	}

	return []Message{{
		ID:        msg.ID,
		From:      a.name,
		To:        "cli",
		Type:      "final_report",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"result":           synthesizeParallelOutput(state),
			"task_id":          taskID,
			"workflow_pattern": "parallel",
		},
	}}, nil
}

func (a *ParallelWorkflowAgent) failureAlert(messageID, taskID, traceID, message string) Message {
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("parallel workflow failed task %s", taskID)
	}

	return Message{
		ID:        messageID,
		From:      a.name,
		To:        "supervisor",
		Type:      TypeSystemAlert,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"severity":         "HIGH",
			"message":          message,
			"task_id":          taskID,
			"trace_id":         traceID,
			"workflow_pattern": "parallel",
		},
	}
}

func (a *ParallelWorkflowAgent) recordTaskCompletion(taskID string, msg Message) (parallelWorkflowState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.states[taskID]
	if !ok || state == nil {
		return parallelWorkflowState{}, false
	}

	index := parsePositiveInt(msg.Payload["subtask_index"], len(state.Results)+1)
	success, _ := msg.Payload["success"].(bool)
	if success {
		output := stringValue(msg.Payload["output"])
		if output == "" && msg.Payload["output"] != nil {
			output = fmt.Sprintf("%v", msg.Payload["output"])
		}
		state.Results = append(state.Results, parallelBranchResult{Index: index, Output: output})
	} else {
		feedback := stringValue(msg.Payload["feedback"])
		if feedback == "" {
			feedback = stringValue(msg.Payload["error"])
		}
		if feedback == "" {
			feedback = fmt.Sprintf("parallel branch %d failed", index)
		}
		branchName := a.lookupBranchName(state, index, stringValue(msg.Payload["branch_name"]))
		state.Errors = append(state.Errors, fmt.Sprintf("%s: %s", branchName, feedback))
	}

	if state.Pending > 0 {
		state.Pending--
	}

	return cloneParallelWorkflowState(state), true
}

func (a *ParallelWorkflowAgent) lookupBranchName(state *parallelWorkflowState, index int, fallback string) string {
	if trimmed := strings.TrimSpace(fallback); trimmed != "" {
		return trimmed
	}
	if state != nil {
		for _, branch := range state.Branches {
			if branch.Index == index && strings.TrimSpace(branch.Name) != "" {
				return branch.Name
			}
		}
	}
	return fmt.Sprintf("branch %d", index)
}

func (a *ParallelWorkflowAgent) deleteState(taskID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.states, taskID)
}

func cloneParallelWorkflowState(state *parallelWorkflowState) parallelWorkflowState {
	if state == nil {
		return parallelWorkflowState{}
	}

	cloned := *state
	cloned.Branches = append([]parallelBranchSpec(nil), state.Branches...)
	cloned.Results = append([]parallelBranchResult(nil), state.Results...)
	cloned.Errors = append([]string(nil), state.Errors...)
	return cloned
}

func resolveParallelBranches(task string, rawValues ...any) []parallelBranchSpec {
	parsed := make([]parallelBranchSpec, 0)
	for _, raw := range rawValues {
		for _, branch := range taskdomain.NormalizeParallelBranches(raw) {
			parsed = append(parsed, parallelBranchSpec{
				Name: branchDisplayName(firstNonEmptyString(branch.Name, branch.Task), len(parsed)+1),
				Task: branch.Task,
			})
		}
	}
	if len(parsed) > 0 {
		return normalizeParallelBranchIndexes(parsed)
	}

	return []parallelBranchSpec{
		{
			Index: 1,
			Name:  "Analysis",
			Task:  fmt.Sprintf("Analyze the task, architecture constraints, dependencies, and success criteria:\n%s", task),
		},
		{
			Index: 2,
			Name:  "Implementation",
			Task:  fmt.Sprintf("Produce the core implementation approach or draft for this task:\n%s", task),
		},
		{
			Index: 3,
			Name:  "Verification",
			Task:  fmt.Sprintf("Produce validation steps, tests, and risk checks for this task:\n%s", task),
		},
	}
}

func normalizeParallelBranchIndexes(branches []parallelBranchSpec) []parallelBranchSpec {
	result := make([]parallelBranchSpec, 0, len(branches))
	for _, branch := range branches {
		task := strings.TrimSpace(branch.Task)
		if task == "" {
			continue
		}
		result = append(result, parallelBranchSpec{
			Index: len(result) + 1,
			Name:  branchDisplayName(branch.Name, len(result)+1),
			Task:  task,
		})
	}
	return result
}

func branchDisplayName(name string, index int) string {
	name = strings.TrimSpace(name)
	if name != "" {
		if len(name) > 48 {
			return strings.TrimSpace(name[:48]) + "..."
		}
		return name
	}
	return fmt.Sprintf("Branch %d", index)
}

func synthesizeParallelOutput(state parallelWorkflowState) string {
	orderedBranches := append([]parallelBranchSpec(nil), state.Branches...)
	sort.Slice(orderedBranches, func(i, j int) bool {
		return orderedBranches[i].Index < orderedBranches[j].Index
	})

	resultByIndex := make(map[int]string, len(state.Results))
	for _, result := range state.Results {
		resultByIndex[result.Index] = strings.TrimSpace(result.Output)
	}

	if len(orderedBranches) == 1 {
		return resultByIndex[orderedBranches[0].Index]
	}

	var builder strings.Builder
	builder.WriteString("Parallel workflow synthesis")
	if strings.TrimSpace(state.Task) != "" {
		builder.WriteString(" for task: ")
		builder.WriteString(state.Task)
	}

	for _, branch := range orderedBranches {
		builder.WriteString("\n\n[")
		builder.WriteString(fmt.Sprintf("%d/%d", branch.Index, len(orderedBranches)))
		builder.WriteString("] ")
		builder.WriteString(branch.Name)
		builder.WriteString("\n")
		builder.WriteString(resultByIndex[branch.Index])
	}

	return builder.String()
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if str, ok := value.(string); ok && str != "" {
			return str
		}
	}
	return ""
}

var _ Agent = (*ParallelWorkflowAgent)(nil)

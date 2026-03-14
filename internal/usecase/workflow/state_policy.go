package workflow

import (
	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
)

type TaskState struct {
	Status       taskdomain.Status
	CurrentStage string
	FinalOutput  string
	ErrorSummary string
}

type TransitionPolicy interface {
	Pattern() taskdomain.WorkflowPattern
	InitialState() TaskState
	Apply(task *taskdomain.Task, msg agentdomain.Message) bool
}

type TransitionRegistry struct {
	policies map[taskdomain.WorkflowPattern]TransitionPolicy
}

func NewTransitionRegistry(policies ...TransitionPolicy) *TransitionRegistry {
	registry := &TransitionRegistry{
		policies: make(map[taskdomain.WorkflowPattern]TransitionPolicy, len(policies)),
	}

	for _, policy := range policies {
		if policy == nil {
			continue
		}
		registry.policies[policy.Pattern()] = policy
	}

	return registry
}

func (r *TransitionRegistry) Supports(pattern taskdomain.WorkflowPattern) bool {
	if r == nil {
		return false
	}
	_, ok := r.policies[taskdomain.NormalizeWorkflowPattern(pattern)]
	return ok
}

func (r *TransitionRegistry) InitialState(pattern taskdomain.WorkflowPattern) (TaskState, bool) {
	if r == nil {
		return TaskState{}, false
	}
	policy, ok := r.policies[taskdomain.NormalizeWorkflowPattern(pattern)]
	if !ok {
		return TaskState{}, false
	}
	return policy.InitialState(), true
}

func (r *TransitionRegistry) Apply(task *taskdomain.Task, msg agentdomain.Message) bool {
	if r == nil || task == nil {
		return false
	}
	policy, ok := r.policies[taskdomain.NormalizeWorkflowPattern(task.WorkflowPattern)]
	if !ok {
		return false
	}
	return policy.Apply(task, msg)
}

type SequentialPolicy struct{}

func NewSequentialPolicy() *SequentialPolicy {
	return &SequentialPolicy{}
}

type ParallelPolicy struct{}

func NewParallelPolicy() *ParallelPolicy {
	return &ParallelPolicy{}
}

func (p *SequentialPolicy) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternSequential
}

func (p *ParallelPolicy) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternParallel
}

func (p *SequentialPolicy) InitialState() TaskState {
	return TaskState{
		Status:       taskdomain.StatusRunning,
		CurrentStage: agentdomain.SequentialWorkflowAgentName,
	}
}

func (p *SequentialPolicy) Apply(task *taskdomain.Task, msg agentdomain.Message) bool {
	switch msg.Type {
	case agentdomain.TypeWorkflowSequentialStart:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.SequentialWorkflowAgentName
	case "task_plan_request":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "planner"
	case agentdomain.TypePlanGenerated:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.SequentialWorkflowAgentName
	case "instruction":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "coder"
	case agentdomain.TypeDraftGenerated:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.SequentialWorkflowAgentName
	case "review_request":
		task.Status = taskdomain.StatusReviewing
		task.CurrentStage = "reviewer"
	case "review_result":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.SequentialWorkflowAgentName
	case "work_progress":
		task.Status = taskdomain.StatusRunning
		if msg.From != "" {
			task.CurrentStage = msg.From
		}
	case "final_report":
		task.Status = taskdomain.StatusCompleted
		task.CurrentStage = "completed"
		if result, ok := msg.Payload["result"].(string); ok {
			task.FinalOutput = result
		}
	case agentdomain.TypeSystemAlert:
		task.Status = taskdomain.StatusFailed
		task.CurrentStage = "failed"
		if message, ok := msg.Payload["message"].(string); ok {
			task.ErrorSummary = message
		}
	default:
		return false
	}

	return true
}

func (p *ParallelPolicy) InitialState() TaskState {
	return TaskState{
		Status:       taskdomain.StatusRunning,
		CurrentStage: agentdomain.ParallelWorkflowAgentName,
	}
}

func (p *ParallelPolicy) Apply(task *taskdomain.Task, msg agentdomain.Message) bool {
	switch msg.Type {
	case agentdomain.TypeWorkflowParallelStart:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.ParallelWorkflowAgentName
	case "task.assigned":
		task.Status = taskdomain.StatusRunning
		if msg.To != "" {
			task.CurrentStage = msg.To
		} else {
			task.CurrentStage = "operational"
		}
	case "task.completed":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.ParallelWorkflowAgentName
	case "work_progress":
		task.Status = taskdomain.StatusRunning
		if msg.From != "" {
			task.CurrentStage = msg.From
		}
	case "final_report":
		task.Status = taskdomain.StatusCompleted
		task.CurrentStage = "completed"
		if result, ok := msg.Payload["result"].(string); ok {
			task.FinalOutput = result
		}
	case agentdomain.TypeSystemAlert:
		task.Status = taskdomain.StatusFailed
		task.CurrentStage = "failed"
		if message, ok := msg.Payload["message"].(string); ok {
			task.ErrorSummary = message
		}
	default:
		return false
	}

	return true
}

type CoordinatorPolicy struct{}

func NewCoordinatorPolicy() *CoordinatorPolicy {
	return &CoordinatorPolicy{}
}

func (p *CoordinatorPolicy) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternCoordinator
}

func (p *CoordinatorPolicy) InitialState() TaskState {
	return TaskState{
		Status:       taskdomain.StatusRunning,
		CurrentStage: agentdomain.CoordinatorWorkflowAgentName,
	}
}

func (p *CoordinatorPolicy) Apply(task *taskdomain.Task, msg agentdomain.Message) bool {
	switch msg.Type {
	case agentdomain.TypeWorkflowCoordinatorStart:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.CoordinatorWorkflowAgentName
	case "milestone.assigned":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "tactical_manager"
	case "task.assigned":
		task.Status = taskdomain.StatusRunning
		if msg.To != "" {
			task.CurrentStage = msg.To
		} else {
			task.CurrentStage = "operational"
		}
	case "task.completed":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "tactical_manager"
	case agentdomain.TypeCoordinationResult:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.CoordinatorWorkflowAgentName
	case "work_progress":
		task.Status = taskdomain.StatusRunning
		if msg.From != "" {
			task.CurrentStage = msg.From
		}
	case "final_report":
		task.Status = taskdomain.StatusCompleted
		task.CurrentStage = "completed"
		if result, ok := msg.Payload["result"].(string); ok {
			task.FinalOutput = result
		}
	case agentdomain.TypeSystemAlert:
		task.Status = taskdomain.StatusFailed
		task.CurrentStage = "failed"
		if message, ok := msg.Payload["message"].(string); ok {
			task.ErrorSummary = message
		}
	default:
		return false
	}

	return true
}

type HierarchicalPolicy struct{}

func NewHierarchicalPolicy() *HierarchicalPolicy {
	return &HierarchicalPolicy{}
}

func (p *HierarchicalPolicy) Pattern() taskdomain.WorkflowPattern {
	return taskdomain.PatternHierarchical
}

func (p *HierarchicalPolicy) InitialState() TaskState {
	return TaskState{
		Status:       taskdomain.StatusRunning,
		CurrentStage: agentdomain.HierarchicalWorkflowAgentName,
	}
}

func (p *HierarchicalPolicy) Apply(task *taskdomain.Task, msg agentdomain.Message) bool {
	switch msg.Type {
	case agentdomain.TypeWorkflowHierarchicalStart:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.HierarchicalWorkflowAgentName
	case "goal.assigned":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "strategic_director"
	case "milestone.assigned":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "tactical_manager"
	case "task.assigned":
		task.Status = taskdomain.StatusRunning
		if msg.To != "" {
			task.CurrentStage = msg.To
		} else {
			task.CurrentStage = "operational"
		}
	case "task.completed":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "tactical_manager"
	case "milestone.feedback":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "strategic_director"
	case agentdomain.TypeGoalResult:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = agentdomain.HierarchicalWorkflowAgentName
	case "work_progress":
		task.Status = taskdomain.StatusRunning
		if msg.From != "" {
			task.CurrentStage = msg.From
		}
	case "final_report":
		task.Status = taskdomain.StatusCompleted
		task.CurrentStage = "completed"
		if result, ok := msg.Payload["result"].(string); ok {
			task.FinalOutput = result
		}
	case agentdomain.TypeSystemAlert:
		task.Status = taskdomain.StatusFailed
		task.CurrentStage = "failed"
		if message, ok := msg.Payload["message"].(string); ok {
			task.ErrorSummary = message
		}
	default:
		return false
	}

	return true
}

type loopPolicyDefinition struct {
	pattern      taskdomain.WorkflowPattern
	agentName    string
	startType    string
	approvalStay taskdomain.Status
}

type LoopPolicy struct {
	definition loopPolicyDefinition
}

func NewLoopPolicy() *LoopPolicy {
	return &LoopPolicy{definition: loopPolicyDefinition{
		pattern:      taskdomain.PatternLoop,
		agentName:    agentdomain.LoopWorkflowAgentName,
		startType:    agentdomain.TypeWorkflowLoopStart,
		approvalStay: taskdomain.StatusRunning,
	}}
}

func NewReviewCritiquePolicy() *LoopPolicy {
	return &LoopPolicy{definition: loopPolicyDefinition{
		pattern:      taskdomain.PatternReviewCritique,
		agentName:    agentdomain.ReviewCritiqueWorkflowAgentName,
		startType:    agentdomain.TypeWorkflowReviewCritiqueStart,
		approvalStay: taskdomain.StatusReviewing,
	}}
}

func NewIterativeRefinementPolicy() *LoopPolicy {
	return &LoopPolicy{definition: loopPolicyDefinition{
		pattern:      taskdomain.PatternIterativeRefinement,
		agentName:    agentdomain.IterativeRefinementWorkflowAgentName,
		startType:    agentdomain.TypeWorkflowIterativeStart,
		approvalStay: taskdomain.StatusRunning,
	}}
}

func (p *LoopPolicy) Pattern() taskdomain.WorkflowPattern {
	if p == nil {
		return ""
	}
	return p.definition.pattern
}

func (p *LoopPolicy) InitialState() TaskState {
	return TaskState{
		Status:       taskdomain.StatusRunning,
		CurrentStage: p.definition.agentName,
	}
}

func (p *LoopPolicy) Apply(task *taskdomain.Task, msg agentdomain.Message) bool {
	switch msg.Type {
	case p.definition.startType:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = p.definition.agentName
	case "instruction":
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = "coder"
	case agentdomain.TypeDraftGenerated:
		task.Status = taskdomain.StatusRunning
		task.CurrentStage = p.definition.agentName
	case "review_request":
		task.Status = taskdomain.StatusReviewing
		task.CurrentStage = "reviewer"
	case "review_result":
		approved, _ := msg.Payload["approved"].(bool)
		if approved {
			task.Status = p.definition.approvalStay
		} else {
			task.Status = taskdomain.StatusRunning
		}
		task.CurrentStage = p.definition.agentName
	case "work_progress":
		task.Status = taskdomain.StatusRunning
		if msg.From != "" {
			task.CurrentStage = msg.From
		}
	case "final_report":
		task.Status = taskdomain.StatusCompleted
		task.CurrentStage = "completed"
		if result, ok := msg.Payload["result"].(string); ok {
			task.FinalOutput = result
		}
	case agentdomain.TypeSystemAlert:
		task.Status = taskdomain.StatusFailed
		task.CurrentStage = "failed"
		if message, ok := msg.Payload["message"].(string); ok {
			task.ErrorSummary = message
		}
	default:
		return false
	}

	return true
}

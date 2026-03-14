package workflow

import (
	"testing"

	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
)

func TestTransitionRegistryInitialStates(t *testing.T) {
	registry := NewTransitionRegistry(
		NewSequentialPolicy(),
		NewParallelPolicy(),
		NewLoopPolicy(),
		NewCoordinatorPolicy(),
		NewHierarchicalPolicy(),
		NewReviewCritiquePolicy(),
		NewIterativeRefinementPolicy(),
	)

	sequentialState, ok := registry.InitialState(taskdomain.PatternSequential)
	if !ok {
		t.Fatal("expected sequential initial state")
	}
	if sequentialState.Status != taskdomain.StatusRunning || sequentialState.CurrentStage != agentdomain.SequentialWorkflowAgentName {
		t.Fatalf("unexpected sequential initial state: %#v", sequentialState)
	}

	parallelState, ok := registry.InitialState(taskdomain.PatternParallel)
	if !ok {
		t.Fatal("expected parallel initial state")
	}
	if parallelState.Status != taskdomain.StatusRunning || parallelState.CurrentStage != agentdomain.ParallelWorkflowAgentName {
		t.Fatalf("unexpected parallel initial state: %#v", parallelState)
	}

	loopState, ok := registry.InitialState(taskdomain.PatternLoop)
	if !ok {
		t.Fatal("expected loop initial state")
	}
	if loopState.Status != taskdomain.StatusRunning || loopState.CurrentStage != agentdomain.LoopWorkflowAgentName {
		t.Fatalf("unexpected loop initial state: %#v", loopState)
	}

	reviewState, ok := registry.InitialState(taskdomain.PatternReviewCritique)
	if !ok {
		t.Fatal("expected review_critique initial state")
	}
	if reviewState.Status != taskdomain.StatusRunning || reviewState.CurrentStage != agentdomain.ReviewCritiqueWorkflowAgentName {
		t.Fatalf("unexpected review_critique initial state: %#v", reviewState)
	}

	iterativeState, ok := registry.InitialState(taskdomain.PatternIterativeRefinement)
	if !ok {
		t.Fatal("expected iterative_refinement initial state")
	}
	if iterativeState.Status != taskdomain.StatusRunning || iterativeState.CurrentStage != agentdomain.IterativeRefinementWorkflowAgentName {
		t.Fatalf("unexpected iterative_refinement initial state: %#v", iterativeState)
	}

	coordinatorState, ok := registry.InitialState(taskdomain.PatternCoordinator)
	if !ok {
		t.Fatal("expected coordinator initial state")
	}
	if coordinatorState.Status != taskdomain.StatusRunning || coordinatorState.CurrentStage != agentdomain.CoordinatorWorkflowAgentName {
		t.Fatalf("unexpected coordinator initial state: %#v", coordinatorState)
	}

	hierarchicalState, ok := registry.InitialState(taskdomain.PatternHierarchical)
	if !ok {
		t.Fatal("expected hierarchical initial state")
	}
	if hierarchicalState.Status != taskdomain.StatusRunning || hierarchicalState.CurrentStage != agentdomain.HierarchicalWorkflowAgentName {
		t.Fatalf("unexpected hierarchical initial state: %#v", hierarchicalState)
	}
}

func TestTransitionRegistryAppliesPatternSpecificStateMachine(t *testing.T) {
	registry := NewTransitionRegistry(
		NewSequentialPolicy(),
		NewParallelPolicy(),
		NewLoopPolicy(),
		NewCoordinatorPolicy(),
		NewHierarchicalPolicy(),
		NewReviewCritiquePolicy(),
		NewIterativeRefinementPolicy(),
	)

	sequentialTask := &taskdomain.Task{WorkflowPattern: taskdomain.PatternSequential}
	changed := registry.Apply(sequentialTask, agentdomain.Message{
		Type: "review_result",
		Payload: map[string]any{
			"approved": false,
		},
	})
	if !changed {
		t.Fatal("expected sequential review_result to change state")
	}
	if sequentialTask.CurrentStage != agentdomain.SequentialWorkflowAgentName || sequentialTask.Status != taskdomain.StatusRunning {
		t.Fatalf("unexpected sequential transition: %#v", sequentialTask)
	}

	parallelTask := &taskdomain.Task{WorkflowPattern: taskdomain.PatternParallel}
	changed = registry.Apply(parallelTask, agentdomain.Message{
		Type: "task.completed",
	})
	if !changed {
		t.Fatal("expected parallel task.completed to change state")
	}
	if parallelTask.CurrentStage != agentdomain.ParallelWorkflowAgentName || parallelTask.Status != taskdomain.StatusRunning {
		t.Fatalf("unexpected parallel transition: %#v", parallelTask)
	}

	loopTask := &taskdomain.Task{WorkflowPattern: taskdomain.PatternLoop}
	changed = registry.Apply(loopTask, agentdomain.Message{
		Type: "review_result",
		Payload: map[string]any{
			"approved": false,
		},
	})
	if !changed {
		t.Fatal("expected loop review_result to change state")
	}
	if loopTask.CurrentStage != agentdomain.LoopWorkflowAgentName || loopTask.Status != taskdomain.StatusRunning {
		t.Fatalf("unexpected loop transition: %#v", loopTask)
	}

	reviewTask := &taskdomain.Task{WorkflowPattern: taskdomain.PatternReviewCritique}
	changed = registry.Apply(reviewTask, agentdomain.Message{
		Type: "review_result",
		Payload: map[string]any{
			"approved": false,
		},
	})
	if !changed {
		t.Fatal("expected review_critique review_result to change state")
	}
	if reviewTask.CurrentStage != agentdomain.ReviewCritiqueWorkflowAgentName || reviewTask.Status != taskdomain.StatusRunning {
		t.Fatalf("unexpected review_critique transition: %#v", reviewTask)
	}

	iterativeTask := &taskdomain.Task{WorkflowPattern: taskdomain.PatternIterativeRefinement}
	changed = registry.Apply(iterativeTask, agentdomain.Message{
		Type: "review_result",
		Payload: map[string]any{
			"approved": false,
		},
	})
	if !changed {
		t.Fatal("expected iterative_refinement review_result to change state")
	}
	if iterativeTask.CurrentStage != agentdomain.IterativeRefinementWorkflowAgentName || iterativeTask.Status != taskdomain.StatusRunning {
		t.Fatalf("unexpected iterative_refinement transition: %#v", iterativeTask)
	}

	coordinatorTask := &taskdomain.Task{WorkflowPattern: taskdomain.PatternCoordinator}
	changed = registry.Apply(coordinatorTask, agentdomain.Message{
		Type: agentdomain.TypeCoordinationResult,
	})
	if !changed {
		t.Fatal("expected coordinator coordination.result to change state")
	}
	if coordinatorTask.CurrentStage != agentdomain.CoordinatorWorkflowAgentName || coordinatorTask.Status != taskdomain.StatusRunning {
		t.Fatalf("unexpected coordinator transition: %#v", coordinatorTask)
	}

	hierarchicalTask := &taskdomain.Task{WorkflowPattern: taskdomain.PatternHierarchical}
	changed = registry.Apply(hierarchicalTask, agentdomain.Message{
		Type: agentdomain.TypeGoalResult,
	})
	if !changed {
		t.Fatal("expected hierarchical goal.result to change state")
	}
	if hierarchicalTask.CurrentStage != agentdomain.HierarchicalWorkflowAgentName || hierarchicalTask.Status != taskdomain.StatusRunning {
		t.Fatalf("unexpected hierarchical transition: %#v", hierarchicalTask)
	}
}

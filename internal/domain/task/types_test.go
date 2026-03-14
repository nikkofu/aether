package task

import "testing"

func TestNormalizeWorkflowPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    WorkflowPattern
		expected WorkflowPattern
		valid    bool
	}{
		{name: "default empty", input: "", expected: PatternSequential, valid: true},
		{name: "trim and lowercase native pattern", input: " Parallel ", expected: PatternParallel, valid: true},
		{name: "legacy prompt chaining", input: "prompt_chaining", expected: PatternSequential, valid: true},
		{name: "trim and lowercase legacy alias", input: " Evaluator_Optimizer ", expected: PatternReviewCritique, valid: true},
		{name: "legacy orchestrator workers", input: "orchestrator_workers", expected: PatternSequential, valid: true},
		{name: "legacy parallelization", input: "parallelization", expected: PatternParallel, valid: true},
		{name: "legacy routing", input: "routing", expected: PatternCoordinator, valid: true},
		{name: "legacy evaluator optimizer", input: "evaluator_optimizer", expected: PatternReviewCritique, valid: true},
		{name: "native sequential", input: PatternSequential, expected: PatternSequential, valid: true},
		{name: "native parallel", input: PatternParallel, expected: PatternParallel, valid: true},
		{name: "native iterative refinement", input: PatternIterativeRefinement, expected: PatternIterativeRefinement, valid: true},
		{name: "reject single agent", input: WorkflowPattern("single_agent"), expected: WorkflowPattern("single_agent"), valid: false},
		{name: "reject react", input: WorkflowPattern("react"), expected: WorkflowPattern("react"), valid: false},
		{name: "reject swarm", input: WorkflowPattern("swarm"), expected: WorkflowPattern("swarm"), valid: false},
		{name: "reject human in the loop", input: WorkflowPattern("human_in_the_loop"), expected: WorkflowPattern("human_in_the_loop"), valid: false},
		{name: "reject custom logic", input: WorkflowPattern("custom_logic"), expected: WorkflowPattern("custom_logic"), valid: false},
		{name: "invalid", input: "totally_custom", expected: "totally_custom", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := NormalizeWorkflowPattern(tc.input)
			if actual != tc.expected {
				t.Fatalf("expected normalized pattern %s, got %s", tc.expected, actual)
			}
			if IsValidWorkflowPattern(tc.input) != tc.valid {
				t.Fatalf("expected valid=%t for %s", tc.valid, tc.input)
			}
		})
	}
}

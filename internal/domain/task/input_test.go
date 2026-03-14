package task

import (
	"reflect"
	"testing"
)

func TestNormalizeParallelBranches(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []ParallelBranch
	}{
		{
			name:  "from strings",
			input: []string{"Plan::Analyze architecture", "Build::Implement branch fan-out", "Verify::Write rollout"},
			expected: []ParallelBranch{
				{Name: "Plan", Task: "Analyze architecture"},
				{Name: "Build", Task: "Implement branch fan-out"},
				{Name: "Verify", Task: "Write rollout"},
			},
		},
		{
			name: "from anys",
			input: []any{
				" Analyze architecture ",
				map[string]any{"name": "Build", "task": " Implement branch fan-out "},
				map[string]any{"name": " ", "prompt": "Write rollout"},
				"",
				map[string]any{"task": "  "},
			},
			expected: []ParallelBranch{
				{Task: "Analyze architecture"},
				{Name: "Build", Task: "Implement branch fan-out"},
				{Task: "Write rollout"},
			},
		},
		{
			name: "from typed maps",
			input: []map[string]any{
				{"name": "Plan", "task": "Analyze architecture"},
				{"title": "Build", "description": "Implement branch fan-out"},
			},
			expected: []ParallelBranch{
				{Name: "Plan", Task: "Analyze architecture"},
				{Name: "Build", Task: "Implement branch fan-out"},
			},
		},
		{
			name:     "drop empty branches",
			input:    []any{" ", map[string]any{"task": ""}, map[string]any{"name": "Plan"}},
			expected: nil,
		},
		{
			name:  "parse text block",
			input: "Plan::Analyze architecture||Build::Implement branch fan-out\nVerify::Write rollout",
			expected: []ParallelBranch{
				{Name: "Plan", Task: "Analyze architecture"},
				{Name: "Build", Task: "Implement branch fan-out"},
				{Name: "Verify", Task: "Write rollout"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := NormalizeParallelBranches(tc.input)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected %#v, got %#v", tc.expected, actual)
			}
		})
	}
}

func TestNormalizeTaskInputParallelCanonicalizesBranches(t *testing.T) {
	input := map[string]any{
		LegacyParallelTasksInputKey: []any{
			"Plan::Analyze architecture",
			map[string]any{"name": "Build", "task": "Implement branch fan-out"},
			"",
		},
		"retry_of": "task-1",
	}

	actual := NormalizeTaskInput(PatternParallel, input)
	expected := map[string]any{
		ParallelBranchesInputKey: []map[string]any{
			{"name": "Plan", "task": "Analyze architecture"},
			{"name": "Build", "task": "Implement branch fan-out"},
		},
		"retry_of": "task-1",
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected %#v, got %#v", expected, actual)
	}
}

func TestNormalizeTaskInputLoopNormalizesIterationCount(t *testing.T) {
	actual := NormalizeTaskInput(PatternLoop, map[string]any{
		MaxReviewIterationsInputKey: float64(4),
	})

	expected := map[string]any{
		MaxReviewIterationsInputKey: 4,
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected %#v, got %#v", expected, actual)
	}
}

func TestParallelBranchesToInput(t *testing.T) {
	actual := ParallelBranchesToInput([]ParallelBranch{
		{Name: "Plan", Task: "Analyze architecture"},
		{Name: " ", Task: "Implement branch fan-out"},
		{Task: "  "},
	})
	expected := []map[string]any{
		{"name": "Plan", "task": "Analyze architecture"},
		{"task": "Implement branch fan-out"},
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected %#v, got %#v", expected, actual)
	}
}

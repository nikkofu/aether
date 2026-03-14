package main

import (
	"reflect"
	"testing"
)

func TestParseParallelBranchesFlag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []map[string]any
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:  "pipe separated",
			input: "Analyze architecture||Implement branch fan-out||Write verification plan",
			expected: []map[string]any{
				{"task": "Analyze architecture"},
				{"task": "Implement branch fan-out"},
				{"task": "Write verification plan"},
			},
		},
		{
			name:  "newline separated",
			input: "Analyze architecture\nImplement branch fan-out\nWrite verification plan",
			expected: []map[string]any{
				{"task": "Analyze architecture"},
				{"task": "Implement branch fan-out"},
				{"task": "Write verification plan"},
			},
		},
		{
			name:  "named branches",
			input: "Plan::Analyze architecture\nBuild::Implement branch fan-out\nVerify::Write verification plan",
			expected: []map[string]any{
				{"name": "Plan", "task": "Analyze architecture"},
				{"name": "Build", "task": "Implement branch fan-out"},
				{"name": "Verify", "task": "Write verification plan"},
			},
		},
		{
			name:  "trim blanks",
			input: " Analyze architecture ||  || Write verification plan ",
			expected: []map[string]any{
				{"task": "Analyze architecture"},
				{"task": "Write verification plan"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := parseParallelBranchesFlag(tc.input)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected %#v, got %#v", tc.expected, actual)
			}
		})
	}
}

func TestParseTaskDescriptionArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "empty",
			input:    nil,
			expected: "",
		},
		{
			name:     "single token",
			input:    []string{"Implement"},
			expected: "Implement",
		},
		{
			name:     "multi token",
			input:    []string{"Implement", "workflow", "routing"},
			expected: "Implement workflow routing",
		},
		{
			name:     "trim outer blanks",
			input:    []string{"  Implement", "workflow  ", "routing "},
			expected: "Implement workflow   routing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := parseTaskDescriptionArgs(tc.input)
			if actual != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

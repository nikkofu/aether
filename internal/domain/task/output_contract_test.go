package task

import (
	"reflect"
	"testing"
)

func TestExtractOutputContract(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expected    OutputContract
	}{
		{
			name:        "exact bullets and word limit",
			description: "Write exactly 3 bullet points for the release memo. Keep the total under 90 words.",
			expected: OutputContract{
				ExactBulletCount: 3,
				MaxWords:         90,
			},
		},
		{
			name:        "exact bullet prefixes",
			description: "Write exactly 3 bullet points for the release memo. Use these exact bullet prefixes: '- Ship Recommendation:', '- Blockers:', and '- Next Action:'. The '- Blockers:' bullet must mention 'OpenTelemetry collector is not running'.",
			expected: OutputContract{
				ExactBulletCount:    3,
				ExactBulletPrefixes: []string{"- Ship Recommendation:", "- Blockers:", "- Next Action:"},
				BulletPhraseRequirements: []BulletPhraseRequirement{
					{Prefix: "- Blockers:", RequiredPhrase: "OpenTelemetry collector is not running"},
				},
			},
		},
		{
			name:        "checklist count",
			description: "Write a concrete smoke checklist with 5 checks covering daemon health and task events.",
			expected: OutputContract{
				ChecklistCount: 5,
			},
		},
		{
			name:        "no explicit constraints",
			description: "Summarize the release state.",
			expected:    OutputContract{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := ExtractOutputContract(tc.description)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Fatalf("expected %#v, got %#v", tc.expected, actual)
			}
		})
	}
}

func TestValidateOutputAgainstContract(t *testing.T) {
	t.Run("passes compliant bullet output", func(t *testing.T) {
		violations := ValidateOutputAgainstContract(OutputContract{
			ExactBulletCount: 2,
			MaxWords:         12,
		}, "- Runtime path verified\n- Fix telemetry noise next")
		if len(violations) != 0 {
			t.Fatalf("expected no violations, got %#v", violations)
		}
	})

	t.Run("rejects prose when bullets are required", func(t *testing.T) {
		violations := ValidateOutputAgainstContract(OutputContract{
			ExactBulletCount: 2,
		}, "I will do this.\n- Runtime path verified\n- Fix telemetry noise next")
		if len(violations) == 0 {
			t.Fatal("expected violations but got none")
		}
	})

	t.Run("rejects wrong checklist count", func(t *testing.T) {
		violations := ValidateOutputAgainstContract(OutputContract{
			ChecklistCount: 3,
		}, "1. Check health\n2. Submit task")
		if len(violations) == 0 {
			t.Fatal("expected checklist violation but got none")
		}
	})

	t.Run("rejects missing exact bullet prefixes", func(t *testing.T) {
		violations := ValidateOutputAgainstContract(OutputContract{
			ExactBulletPrefixes: []string{"- Ship Recommendation:", "- Blockers:", "- Next Action:"},
		}, "- Release Decision: Hold\n- Blockers: OTel collector is down\n- Next Action: Fix the collector")
		if len(violations) == 0 {
			t.Fatal("expected prefix violation but got none")
		}
	})

	t.Run("rejects bullet missing required phrase", func(t *testing.T) {
		violations := ValidateOutputAgainstContract(OutputContract{
			ExactBulletPrefixes: []string{"- Ship Recommendation:", "- Blockers:", "- Next Action:"},
			BulletPhraseRequirements: []BulletPhraseRequirement{
				{Prefix: "- Blockers:", RequiredPhrase: "OpenTelemetry collector is not running"},
			},
		}, "- Ship Recommendation: Public launch decision\n- Blockers: Daemon health is ok\n- Next Action: Fix the collector")
		if len(violations) == 0 {
			t.Fatal("expected bullet phrase violation but got none")
		}
	})
}

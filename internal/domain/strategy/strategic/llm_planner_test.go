package strategic

import (
	"context"
	"testing"

	"github.com/nikkofu/aether/internal/domain/strategy/evolution"
)

type plannerCapabilityStub struct {
	output string
	err    error
}

func (s *plannerCapabilityStub) Name() string { return "llm" }

func (s *plannerCapabilityStub) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if s.err != nil {
		return nil, s.err
	}
	return map[string]any{"output": s.output}, nil
}

type plannerStrategyEngineStub struct {
	template *evolution.StrategyTemplate
	err      error
}

func (s *plannerStrategyEngineStub) Register(ctx context.Context, template evolution.StrategyTemplate) error {
	return nil
}

func (s *plannerStrategyEngineStub) Activate(ctx context.Context, templateID string, version string) error {
	return nil
}

func (s *plannerStrategyEngineStub) Evaluate(ctx context.Context, templateID string, version string) (float64, error) {
	return 0, nil
}

func (s *plannerStrategyEngineStub) Evolve(ctx context.Context, orgID, templateID string) error {
	return nil
}

func (s *plannerStrategyEngineStub) GetActive(ctx context.Context, orgID string) (*evolution.StrategyTemplate, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.template != nil {
		return s.template, nil
	}
	return &evolution.StrategyTemplate{
		ID:      "tmpl-1",
		Version: "v1",
		Content: "",
		Active:  true,
	}, nil
}

func TestParseGoalPlanDraftsAcceptsStringArray(t *testing.T) {
	drafts, err := parseGoalPlanDrafts(`["Scale delivery","Harden observability"]`)
	if err != nil {
		t.Fatalf("expected string-array goals to parse: %v", err)
	}
	if len(drafts) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(drafts))
	}
	if drafts[0].Title != "Scale delivery" || drafts[0].Description != "Scale delivery" {
		t.Fatalf("unexpected first draft: %#v", drafts[0])
	}
}

func TestParseMilestonePlanDraftsRepairsTrailingNoise(t *testing.T) {
	drafts, err := parseMilestonePlanDrafts("```json\n[{\"title\":\"运行稳定性\"},{\"title\":\"可观测性\"}]\n```\n]\n补充说明")
	if err != nil {
		t.Fatalf("expected milestone drafts to parse: %v", err)
	}
	if len(drafts) != 2 {
		t.Fatalf("expected 2 milestones, got %d", len(drafts))
	}
	if drafts[0].Title != "运行稳定性" {
		t.Fatalf("unexpected first milestone: %#v", drafts[0])
	}
}

func TestParseMilestonePlanDraftsFallsBackToBulletList(t *testing.T) {
	drafts, err := parseMilestonePlanDrafts("- 运行稳定性\n- 可观测性\n- 交付交接")
	if err != nil {
		t.Fatalf("expected bullet-list milestones to parse: %v", err)
	}
	if len(drafts) != 3 {
		t.Fatalf("expected 3 milestones, got %d", len(drafts))
	}
	if drafts[2].Title != "交付交接" {
		t.Fatalf("unexpected third milestone: %#v", drafts[2])
	}
}

func TestPlanMilestonesRepairsBracketNoiseFromLLM(t *testing.T) {
	planner := NewLLMStrategicPlanner(
		&plannerCapabilityStub{
			output: "[{\"title\":\"运行稳定性\"},{\"title\":\"可观测性\"}]]\n补充说明",
		},
		nil,
		&plannerStrategyEngineStub{},
		&testStrategicLogger{},
	)

	milestones, err := planner.PlanMilestones(context.Background(), Goal{
		ID:          "goal-1",
		Title:       "正式发布前验收",
		Description: "正式发布前验收",
	})
	if err != nil {
		t.Fatalf("expected repaired milestones to parse: %v", err)
	}
	if len(milestones) != 2 {
		t.Fatalf("expected 2 milestones, got %d", len(milestones))
	}
	if milestones[0].Title != "运行稳定性" {
		t.Fatalf("unexpected first milestone: %#v", milestones[0])
	}
}

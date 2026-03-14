package learning

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/internal/domain/strategy"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	"github.com/nikkofu/aether/internal/usecase/reflection"
	_ "modernc.org/sqlite"
)

func TestTaskOutcomeObserverPersistsReflectionsAndUpdatesStrategies(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	reflectionStore, err := reflection.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create reflection store: %v", err)
	}
	strategyStore, err := strategy.NewSQLiteStrategyStore(db)
	if err != nil {
		t.Fatalf("failed to create strategy store: %v", err)
	}
	graph, err := knowledge.NewSQLiteGraph(db)
	if err != nil {
		t.Fatalf("failed to create knowledge graph: %v", err)
	}

	observer := NewTaskOutcomeObserver(reflectionStore, NewLearningEngine(strategyStore), graph, nil)
	task := &taskdomain.Task{
		ID:              "task-1",
		WorkflowPattern: taskdomain.PatternReviewCritique,
		Description:     "Write exactly 3 bullet points. Use these exact bullet prefixes: '- Ship Recommendation:', '- Blockers:', and '- Next Action:'. Keep the total under 80 words.",
		Status:          taskdomain.StatusCompleted,
	}
	events := []*taskdomain.Event{
		{
			TaskID: "task-1",
			Type:   "workflow.review_critique.start",
			Payload: map[string]any{
				"org_id": "default",
			},
		},
		{
			TaskID: "task-1",
			From:   "reviewer",
			Type:   "review_result",
			Payload: map[string]any{
				"approved":                     false,
				"quality_gate_violations":      []any{"Bullet 1 must start with the exact prefix `- Ship Recommendation:`.", "The task requires bullet-only output, but the result still contains extra prose or meta commentary."},
				"reviewer_protocol_violations": []any{"The review result is missing an explicit `Decision: [PASS]` or `Decision: [FAIL]` line."},
				"review_decision_source":       "contract_fallback",
			},
		},
		{
			TaskID: "task-1",
			From:   "workflow.review_critique",
			Type:   "final_report",
			Payload: map[string]any{
				"result": "done",
			},
		},
	}

	if err := observer.ObserveTerminalTask(context.Background(), task, events); err != nil {
		t.Fatalf("observe terminal task failed: %v", err)
	}

	reflections, err := reflectionStore.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("failed to list reflections: %v", err)
	}
	if len(reflections) != 2 {
		t.Fatalf("expected 2 reflections, got %d", len(reflections))
	}

	coderStrategy, err := strategyStore.Get("coder")
	if err != nil {
		t.Fatalf("failed to load coder strategy: %v", err)
	}
	if !strings.Contains(coderStrategy.PromptHint, "Satisfy exact bullet prefixes") {
		t.Fatalf("expected coder prompt hint to learn prefix discipline, got %q", coderStrategy.PromptHint)
	}

	reviewerStrategy, err := strategyStore.Get("reviewer")
	if err != nil {
		t.Fatalf("failed to load reviewer strategy: %v", err)
	}
	if !strings.Contains(reviewerStrategy.PromptHint, "Decision: [PASS]") {
		t.Fatalf("expected reviewer prompt hint to learn explicit decision line, got %q", reviewerStrategy.PromptHint)
	}

	graphReflections, err := graph.QueryByType(context.Background(), "default", "reflection")
	if err != nil {
		t.Fatalf("failed to query reflection entities: %v", err)
	}
	if len(graphReflections) != 2 {
		t.Fatalf("expected 2 reflection entities in graph, got %d", len(graphReflections))
	}
}

package observability

import (
	"context"
	"testing"

	obstrace "github.com/nikkofu/aether/pkg/observability/trace"
)

func TestConsoleRendererPersistsSpansToTraceStorage(t *testing.T) {
	storage, err := obstrace.NewSQLiteTraceStorage("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init trace storage: %v", err)
	}
	defer storage.GetDB().Close()

	renderer := NewConsoleRenderer()
	renderer.SetTraceStorage(storage)

	ctx, root := renderer.StartSpan(context.Background(), "root", map[string]any{
		"org_id":     "default",
		"agent_name": "planner",
		"role":       "planner",
	})
	_, child := renderer.StartSpan(ctx, "child", map[string]any{
		"org_id":     "default",
		"agent_name": "coder",
		"role":       "coder",
	})

	child.End()
	root.End()

	traces, err := storage.GetRecentTraces("default", 10)
	if err != nil {
		t.Fatalf("failed to query recent traces: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("expected 1 recent trace, got %d", len(traces))
	}

	traceRecord, err := storage.GetTrace(traces[0].ID)
	if err != nil {
		t.Fatalf("failed to load persisted trace: %v", err)
	}
	if len(traceRecord.Spans) != 2 {
		t.Fatalf("expected 2 persisted spans, got %d", len(traceRecord.Spans))
	}
}

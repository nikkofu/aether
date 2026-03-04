package graph

import (
	"github.com/nikkofu/aether/pkg/observability/trace"
)

type Node struct {
	ID    string         `json:"id"`
	Label string         `json:"label"`
	Type  string         `json:"type"` // Agent, Skill, Capability, External, etc.
	Meta  map[string]any `json:"metadata,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DAG struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// BuildGraph 将 Trace 转换为前端可渲染的 DAG。
func BuildGraph(t *trace.Trace) *DAG {
	dag := &DAG{
		Nodes: make([]Node, 0),
		Edges: make([]Edge, 0),
	}

	for _, s := range t.Spans {
		nodeType := determineNodeType(s)
		dag.Nodes = append(dag.Nodes, Node{
			ID:    s.SpanID,
			Label: s.Action,
			Type:  nodeType,
			Meta:  s.Metadata,
		})

		if s.ParentSpanID != "" {
			dag.Edges = append(dag.Edges, Edge{
				From: s.ParentSpanID,
				To:   s.SpanID,
			})
		}
	}

	return dag
}

func determineNodeType(s *trace.Span) string {
	switch s.Layer {
	case "Strategic", "Tactical", "Operational":
		return "Agent"
	case "Skill":
		return "Skill"
	case "Gateway":
		return "Capability"
	case "Adapter":
		return "External Call"
	default:
		return "Unknown"
	}
}

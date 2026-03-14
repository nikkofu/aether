package main

import (
	"context"
	"testing"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestCaptureServerExportSummarizesRequests(t *testing.T) {
	server := newCaptureServer("")

	_, err := server.Export(context.Background(), &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{
			{
				Resource: &resourcev1.Resource{
					Attributes: []*commonv1.KeyValue{
						{
							Key: "service.name",
							Value: &commonv1.AnyValue{
								Value: &commonv1.AnyValue_StringValue{StringValue: "aether-core"},
							},
						},
					},
				},
				ScopeSpans: []*tracev1.ScopeSpans{
					{
						Spans: []*tracev1.Span{
							{TraceId: []byte{0x01, 0x02, 0x03, 0x04}},
							{TraceId: []byte{0x01, 0x02, 0x03, 0x04}},
							{TraceId: []byte{0xaa, 0xbb, 0xcc, 0xdd}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	summary := server.snapshot()
	if summary.RequestCount != 1 {
		t.Fatalf("expected request count 1, got %d", summary.RequestCount)
	}
	if summary.ResourceSpanCount != 1 {
		t.Fatalf("expected resource span count 1, got %d", summary.ResourceSpanCount)
	}
	if summary.ScopeSpanCount != 1 {
		t.Fatalf("expected scope span count 1, got %d", summary.ScopeSpanCount)
	}
	if summary.SpanCount != 3 {
		t.Fatalf("expected span count 3, got %d", summary.SpanCount)
	}
	if len(summary.Services) != 1 || summary.Services[0] != "aether-core" {
		t.Fatalf("expected service aether-core, got %#v", summary.Services)
	}
	if len(summary.TraceIDs) != 2 {
		t.Fatalf("expected 2 unique trace ids, got %#v", summary.TraceIDs)
	}
}

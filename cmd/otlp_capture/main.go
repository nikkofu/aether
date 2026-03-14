package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
)

type captureSummary struct {
	RequestCount      int      `json:"request_count"`
	ResourceSpanCount int      `json:"resource_span_count"`
	ScopeSpanCount    int      `json:"scope_span_count"`
	SpanCount         int      `json:"span_count"`
	Services          []string `json:"services"`
	TraceIDs          []string `json:"trace_ids"`
	UpdatedAt         string   `json:"updated_at"`
}

type captureServer struct {
	collectortracev1.UnimplementedTraceServiceServer

	mu         sync.Mutex
	summary    captureSummary
	outputPath string
}

func newCaptureServer(outputPath string) *captureServer {
	return &captureServer{
		summary: captureSummary{
			Services: make([]string, 0),
			TraceIDs: make([]string, 0),
		},
		outputPath: outputPath,
	}
}

func (s *captureServer) Export(ctx context.Context, req *collectortracev1.ExportTraceServiceRequest) (*collectortracev1.ExportTraceServiceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.summary.RequestCount++
	s.summary.ResourceSpanCount += len(req.ResourceSpans)
	serviceSet := make(map[string]struct{}, len(s.summary.Services))
	traceSet := make(map[string]struct{}, len(s.summary.TraceIDs))

	for _, service := range s.summary.Services {
		serviceSet[service] = struct{}{}
	}
	for _, traceID := range s.summary.TraceIDs {
		traceSet[traceID] = struct{}{}
	}

	for _, resourceSpan := range req.ResourceSpans {
		serviceName := lookupResourceService(resourceSpan)
		if serviceName != "" {
			if _, exists := serviceSet[serviceName]; !exists {
				serviceSet[serviceName] = struct{}{}
				s.summary.Services = append(s.summary.Services, serviceName)
			}
		}

		for _, scopeSpan := range resourceSpan.ScopeSpans {
			s.summary.ScopeSpanCount++
			s.summary.SpanCount += len(scopeSpan.Spans)
			for _, span := range scopeSpan.Spans {
				traceID := encodeTraceID(span)
				if traceID == "" {
					continue
				}
				if _, exists := traceSet[traceID]; exists {
					continue
				}
				traceSet[traceID] = struct{}{}
				s.summary.TraceIDs = append(s.summary.TraceIDs, traceID)
			}
		}
	}

	s.summary.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.persistSummaryLocked(); err != nil {
		log.Printf("failed to persist capture summary: %v", err)
	}

	return &collectortracev1.ExportTraceServiceResponse{}, nil
}

func (s *captureServer) snapshot() captureSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	clone := s.summary
	clone.Services = append([]string(nil), s.summary.Services...)
	clone.TraceIDs = append([]string(nil), s.summary.TraceIDs...)
	return clone
}

func (s *captureServer) persistSummaryLocked() error {
	if strings.TrimSpace(s.outputPath) == "" {
		return nil
	}

	payload, err := json.MarshalIndent(s.summary, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.outputPath)
}

func lookupResourceService(resourceSpan *tracev1.ResourceSpans) string {
	if resourceSpan == nil || resourceSpan.Resource == nil {
		return ""
	}

	for _, attr := range resourceSpan.Resource.Attributes {
		if attr == nil || attr.Key != "service.name" {
			continue
		}
		return anyValueString(attr.Value)
	}
	return ""
}

func anyValueString(value *commonv1.AnyValue) string {
	if value == nil {
		return ""
	}
	if str := value.GetStringValue(); strings.TrimSpace(str) != "" {
		return strings.TrimSpace(str)
	}
	return ""
}

func encodeTraceID(span *tracev1.Span) string {
	if span == nil || len(span.TraceId) == 0 {
		return ""
	}
	return hex.EncodeToString(span.TraceId)
}

func healthHandler(server *captureServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
		})
	})
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(server.snapshot())
	})
	return mux
}

func main() {
	grpcAddr := envOrDefault("OTLP_CAPTURE_GRPC_ADDR", "127.0.0.1:4317")
	httpAddr := envOrDefault("OTLP_CAPTURE_HTTP_ADDR", "127.0.0.1:18096")
	outputPath := envOrDefault("OTLP_CAPTURE_OUTPUT", "/tmp/aether-otlp-capture.json")

	server := newCaptureServer(outputPath)

	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", grpcAddr, err)
	}
	defer grpcListener.Close()

	grpcServer := grpc.NewServer()
	collectortracev1.RegisterTraceServiceServer(grpcServer, server)

	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: healthHandler(server),
	}

	go func() {
		log.Printf("OTLP capture gRPC listening on %s", grpcAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("grpc serve failed: %v", err)
		}
	}()

	go func() {
		log.Printf("OTLP capture HTTP listening on %s", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http serve failed: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("http shutdown failed: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

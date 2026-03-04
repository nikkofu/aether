package main

import (
	"log"
	"net/http"

	"github.com/nikkofu/aether/internal/pkg/observability/metrics"
	"github.com/nikkofu/aether/internal/pkg/observability/trace"
)

func main() {
	// 初始化存储和引擎
	storage, err := trace.NewSQLiteTraceStorage("aether.db")
	if err != nil {
		log.Fatalf("failed to init storage: %v", err)
	}
	engine := trace.NewTraceEngine(storage)
	metricsEngine := metrics.NewMetricsEngine(storage.GetDB())

	// 设置路由
	mux := setupRoutes(engine, metricsEngine)

	log.Println("Observability API listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

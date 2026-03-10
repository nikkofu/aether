package main

import (
	"log"
	"net/http"
	"os"

	"github.com/nikkofu/aether/pkg/observability/metrics"
	"github.com/nikkofu/aether/pkg/observability/trace"
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

	port := os.Getenv("OBSERVABILITY_API_PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("Observability API listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

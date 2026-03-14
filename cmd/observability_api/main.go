package main

import (
	"log"
	"net/http"
	"os"

	"github.com/nikkofu/aether/pkg/config"
	"github.com/nikkofu/aether/pkg/observability/metrics"
	"github.com/nikkofu/aether/pkg/observability/trace"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dbPath := os.Getenv("OBSERVABILITY_API_DATABASE_PATH")
	if dbPath == "" {
		dbPath = cfg.Runtime.DatabasePath
	}
	if dbPath == "" {
		dbPath = "./aether.db"
	}

	// 初始化存储和引擎
	storage, err := trace.NewSQLiteTraceStorage(dbPath)
	if err != nil {
		log.Fatalf("failed to init storage: %v", err)
	}
	defer storage.GetDB().Close()

	engine := trace.NewTraceEngine(storage)
	metricsEngine := metrics.NewMetricsEngine(storage.GetDB())

	// 设置路由
	mux := setupRoutes(engine, metricsEngine, dbPath)

	port := os.Getenv("OBSERVABILITY_API_PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("Observability API listening on :%s using %s\n", port, dbPath)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

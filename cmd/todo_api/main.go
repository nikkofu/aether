package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"github.com/nikkofu/aether/pkg/config"
	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/internal/delivery/todo"
	_ "modernc.org/sqlite"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 2. 初始化日志
	logger, err := logging.NewLogger(logging.Config{
		Level:  cfg.Log.Level,
		Format: "console",
	})
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logger.Sync()

	ctx := context.Background()
	logger.Info(ctx, "Starting Todo API", logging.String("mode", cfg.App.Mode))

	// 3. 初始化数据库连接
	db, err := sql.Open("sqlite", cfg.Runtime.DatabasePath)
	if err != nil {
		logger.Error(ctx, "failed to open database", logging.Err(err))
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// 4. 初始化存储
	store, err := todo.NewSQLiteStore(db)
	if err != nil {
		logger.Error(ctx, "failed to init todo store", logging.Err(err))
		log.Fatalf("failed to init todo store: %v", err)
	}

	// 5. 初始化处理器
	handler := todo.NewHandler(store, logger)

	// 6. 设置路由
	mux := http.NewServeMux()
	
	// CORS 中间件
	withCORS := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	handler.RegisterRoutes(mux)

	// 7. 启动服务
	addr := ":8081"
	logger.Info(ctx, "Todo API listening", logging.String("address", addr))
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		logger.Error(ctx, "server stopped", logging.Err(err))
		log.Fatal(err)
	}
}

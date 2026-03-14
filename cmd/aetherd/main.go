package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nikkofu/aether/internal/app"
	"github.com/nikkofu/aether/internal/delivery/api"
	"github.com/nikkofu/aether/internal/delivery/webhook"
	"github.com/nikkofu/aether/pkg/config"
	"github.com/nikkofu/aether/pkg/logging"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}

	// 强制以集群 worker 或独立守护进程模式启动
	if cfg.App.Mode == "single" {
		cfg.App.Mode = "cluster-leader" // 守护进程需要能分配任务
	}

	// 2. 初始化核心运行时
	rt := app.NewDefaultRuntime(cfg)
	defer rt.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if rt.TaskService() == nil {
		log.Fatal("task service is not initialized")
	}

	if recovered, err := rt.TaskService().RecoverInterrupted(ctx); err != nil {
		log.Fatalf("failed to recover interrupted tasks: %v", err)
	} else if recovered > 0 {
		rt.Logger().Warn(ctx, "Recovered interrupted tasks", logging.Int("count", recovered))
	}

	// 3. 启动所有总线订阅和系统代理
	go rt.StartAgents(ctx)

	// 4. 配置 Webhook HTTP Server
	mux := http.NewServeMux()

	taskHandler := api.NewTaskHandler(rt.TaskService(), rt.Logger())
	taskHandler.RegisterRoutes(mux)

	systemHandler := api.NewSystemHandler(rt.TaskService(), rt.GetBus(), rt.AgentManager(), rt.Logger())
	systemHandler.RegisterRoutes(mux)

	// 注册 GitHub Webhook Handler
	deliveryStore, err := webhook.NewSQLiteDeliveryStore(rt.DB())
	if err != nil {
		log.Fatalf("failed to initialize webhook delivery store: %v", err)
	}

	ghHandler := webhook.NewGitHubWebhookHandler(
		rt.TaskService(),
		deliveryStore,
		os.Getenv("AETHER_GITHUB_WEBHOOK_SECRET"),
		rt.Logger(),
	)
	mux.HandleFunc("/webhooks/github", ghHandler.Handle)

	// 注册实时事件流 Handler
	streamHandler := api.NewStreamHandler(rt.GetBus(), rt.Logger())
	mux.HandleFunc("/stream", streamHandler.Handle)

	port := os.Getenv("AETHERD_PORT")
	if port == "" {
		port = "8090"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: withCORS(mux),
	}

	// 5. 启动服务器
	go func() {
		rt.Logger().Info(ctx, fmt.Sprintf("Aether Daemon 已启动，正在监听端口 %s...", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 6. 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	rt.Logger().Info(ctx, "正在关闭 Aether Daemon...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		rt.Logger().Error(ctx, "Server shutdown error", logging.Err(err))
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

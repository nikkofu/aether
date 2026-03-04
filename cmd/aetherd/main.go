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

	// 3. 启动所有总线订阅和系统代理
	go rt.StartAgents(ctx)

	// 4. 配置 Webhook HTTP Server
	mux := http.NewServeMux()
	
	// 注册 GitHub Webhook Handler
	ghHandler := webhook.NewGitHubWebhookHandler(rt.GetBus(), rt.Logger())
	mux.HandleFunc("/webhooks/github", ghHandler.Handle)

	port := os.Getenv("AETHERD_PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
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

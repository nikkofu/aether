package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/pkg/observability/otel" // 使用项目自带的初始化
	go_otel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func main() {
	ctx := context.Background()

	// 1. 使用项目标准初始化，发送到 localhost:4317 (Jaeger gRPC)
	// 设置服务名为 aether-core，方便你在 Jaeger 下拉框找到它
	shutdown, err := otel.InitTracer("aether-core")
	if err != nil {
		fmt.Printf("❌ 初始化失败: %v\n", err)
		return
	}
	defer shutdown(ctx)

	logger, _ := logging.NewLogger(logging.Config{Level: "info", Format: "console"})
	fmt.Println("\n🚀 [Aether] 正在将自愈模拟数据发送至 Jaeger (localhost:16686)...")

	tracer := go_otel.Tracer("demo-tracer")
	ctx, span := tracer.Start(ctx, "Self-Healing-Workflow")
	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	fmt.Printf("🔍 当前 TraceID: %s\n", traceID)
	fmt.Printf("🔗 请刷新 Jaeger: http://localhost:16686/trace/%s\n\n", traceID)

	// 2. 模拟任务过程
	simulateTask(ctx, logger)

	// 强制等待一秒，确保 BatchSpanProcessor 完成发送
	fmt.Println("⏳ 正在同步 Trace 数据到 Jaeger 存储层...")
	time.Sleep(2 * time.Second)
	fmt.Println("✅ 任务结束。现在你应该能在 Jaeger 中搜索到刚才的结果了！")
}

func simulateTask(ctx context.Context, logger logging.Logger) {
	tracer := go_otel.Tracer("demo-tracer")
	ctx, span := tracer.Start(ctx, "Agent.Reasoning.Loop")
	defer span.End()

	// 故意制造一个坏的 JSON
	badJSON := `{"task": "generate_report", "format": "pdf", [CORRUPT_DATA]`
	logger.Info(ctx, "Agent 开始思维链路推理...", logging.String("step", "planning"))

	var result map[string]any
	err := json.Unmarshal([]byte(badJSON), &result)
	if err != nil {
		// ⚠️ 重点：将错误和现场数据注入到 Jaeger
		span.RecordError(err)
		span.SetStatus(codes.Error, "JSON_PARSING_FAILED")
		span.SetAttributes(
			attribute.String("llm.raw_response", badJSON),
			attribute.String("recovery.strategy", "Reflection_Retry"),
		)

		logger.Error(ctx, "CRITICAL: Agent 决策解析失败！", 
			logging.Err(err), 
			logging.String("trace_id", span.SpanContext().TraceID().String()),
		)

		// 模拟自愈过程
		fmt.Println("\n🛠️  [自愈系统] 正在从 Trace 历史中回溯故障现场...")
		fmt.Printf("💡 [自愈系统] 成功提取 Raw Response: %s\n", badJSON)
		
		correctedJSON := `{"task": "generate_report", "format": "pdf", "status": "fixed"}`
		logger.Info(ctx, "自愈成功！", logging.String("new_response", correctedJSON))
	}
}

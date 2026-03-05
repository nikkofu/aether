package otel

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// InitTracer 初始化全局 OpenTelemetry 追踪器。
// 返回一个用于优雅关闭的清理函数。
func InitTracer(serviceName string) (func(context.Context) error, error) {
	ctx := context.Background()

	// 1. 从环境变量读取 OTEL_EXPORTER_OTLP_ENDPOINT
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	// 2. 配置 OTLP gRPC 导出器 (增加重试和超时控制)
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithReconnectionPeriod(2*time.Second),
		otlptracegrpc.WithTimeout(500*time.Millisecond), // 强制极短超时
	)

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create otlp trace exporter: %w", err)
	}

	// 3. 设置资源标签 (Resources)
	nodeID, _ := os.Hostname()
	if nodeID == "" {
		nodeID = uuid.New().String()
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.HostNameKey.String(nodeID),
			// 自定义 node.id 标签
			semconv.ServiceInstanceIDKey.String(nodeID),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 4. 配置 TracerProvider
	// 使用 BatchSpanProcessor 优化性能 (生产环境必备)
	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // 采样策略可根据负载调整为 ParentBased(AlwaysSample)
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局传播器 (用于分布式链路 Context 注入与提取)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 5. 返回优雅关闭函数
	shutdown := func(ctx context.Context) error {
		// 延迟一秒以确保最后一批数据进入发送队列
		time.Sleep(1 * time.Second)
		
		// 设定关闭超时时间
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := tp.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shutdown tracer provider: %w", err)
		}
		return nil
	}

	return shutdown, nil
}

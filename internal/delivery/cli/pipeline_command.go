package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nikkofu/aether/internal/usecase/dag"
	"github.com/nikkofu/aether/internal/app"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// PipelineHandler 处理与流水线相关的命令行指令。
type PipelineHandler struct {
	runtime *app.Runtime
}

// NewPipelineHandler 创建一个新的 PipelineHandler。
func NewPipelineHandler(rt *app.Runtime) *PipelineHandler {
	return &PipelineHandler{
		runtime: rt,
	}
}

// Handle 执行 pipeline 子命令的分发。
func (h *PipelineHandler) Handle(ctx context.Context, args []string) {
	if len(args) < 1 {
		h.printUsage()
		os.Exit(1)
	}

	subCommand := args[0]
	subArgs := args[1:]

	switch subCommand {
	case "run":
		h.runPipeline(ctx, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知 pipeline 子命令 '%s'\n", subCommand)
		h.printUsage()
		os.Exit(1)
	}
}

// runPipeline 读取 YAML 配置文件并执行流水线。
func (h *PipelineHandler) runPipeline(ctx context.Context, args []string) {
	runCmd := flag.NewFlagSet("pipeline run", flag.ExitOnError)
	inputStr := runCmd.String("input", "", "JSON 格式的输入参数")
	runCmd.Parse(args)

	remainingArgs := runCmd.Args()
	if len(remainingArgs) < 1 {
		fmt.Fprintln(os.Stderr, "错误: 必须指定流水线配置文件路径 (.yaml)")
		h.printUsage()
		os.Exit(1)
	}
	filePath := remainingArgs[0]

	// 1. Tracing 增强
	tracer := otel.Tracer("aether-cli")
	ctx, span := tracer.Start(ctx, "cli.pipeline_run")
	span.SetAttributes(
		attribute.String("pipeline.file", filePath),
		attribute.String("pipeline.input", *inputStr),
	)
	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	fmt.Fprintf(os.Stderr, "🔍 追踪已启动 (TraceID: %s)\n", traceID)
	fmt.Fprintf(os.Stderr, "🔗 在 Jaeger 中查看: http://localhost:16686/trace/%s\n", traceID)

	// 2. 读取并解析 YAML
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		h.printErrorJSON(fmt.Errorf("读取文件失败: %w", err))
		os.Exit(1)
	}

	var pipeline dag.Pipeline
	if err := yaml.Unmarshal(fileBytes, &pipeline); err != nil {
		h.printErrorJSON(fmt.Errorf("解析 YAML 失败: %w", err))
		os.Exit(1)
	}

	// 3. 处理参数化输入：如果提供了 --input，则覆盖 Pipeline 的 InitialData
	if *inputStr != "" {
		var inputData map[string]any
		if err := json.Unmarshal([]byte(*inputStr), &inputData); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  警告: --input 解析 JSON 失败: %v\n", err)
		} else {
			if pipeline.InitialData == nil {
				pipeline.InitialData = make(map[string]any)
			}
			for k, v := range inputData {
				pipeline.InitialData[k] = v
			}
			fmt.Fprintf(os.Stderr, "✅ 已加载自定义输入参数: %d 个字段\n", len(inputData))
		}
	}

	// 4. 使用 Runtime 创建带策略的执行器
	executor := h.runtime.NewPipelineExecutor(5)

	// 5. 启动事件监听 Goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	startTime := time.Now()

	go func() {
		defer wg.Done()
		for event := range executor.Events() {
			switch event.Type {
			case dag.EventNodeStarted:
				fmt.Fprintf(os.Stderr, "%-10s %s\n", "[START]", event.NodeID)
			case dag.EventNodeCompleted:
				fmt.Fprintf(os.Stderr, "%-10s %-20s (%s)\n", "[DONE]", event.NodeID, event.Duration.Round(time.Millisecond))
			case dag.EventNodeFailed:
				fmt.Fprintf(os.Stderr, "%-10s %-20s 错误: %v\n", "[FAIL]", event.NodeID, event.Error)
			case dag.EventPipelineStarted:
				fmt.Fprintln(os.Stderr, ">>> 流水线启动任务执行...")
			case dag.EventPipelineCompleted:
				duration := time.Since(startTime).Round(time.Millisecond)
				fmt.Fprintf(os.Stderr, ">>> 流水线执行结束，总耗时: %s\n", duration)
			}
		}
	}()

	// 6. 执行流水线
	results, err := executor.Execute(ctx, &pipeline)
	
	// 等待事件监听协程处理完所有剩余事件
	wg.Wait()

	if err != nil {
		h.printErrorJSON(fmt.Errorf("流水线执行失败: %w", err))
		os.Exit(1)
	}

	// 7. 以 JSON 格式打印最终结果到 Stdout
	h.printJSON(map[string]any{
		"status":   "success",
		"trace_id": traceID,
		"results":  results,
	})
}

func (h *PipelineHandler) printJSON(data any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 编码失败: %v\n", err)
	}
}

func (h *PipelineHandler) printErrorJSON(err error) {
	h.printJSON(map[string]any{
		"status": "error",
		"error":  err.Error(),
	})
}

func (h *PipelineHandler) printUsage() {
	fmt.Println("用法: aether pipeline <command> [arguments]")
	fmt.Println("\n可用命令:")
	fmt.Println("  run <file.yaml> 执行指定的流水线配置文件")
}

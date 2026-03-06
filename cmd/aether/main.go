package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/delivery/cli"
	"github.com/nikkofu/aether/internal/usecase/cluster"
	"github.com/nikkofu/aether/pkg/config"
	"github.com/nikkofu/aether/pkg/observability/otel"
	"github.com/nikkofu/aether/internal/app"
	go_otel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func main() {
	// 初始化 OpenTelemetry
	shutdown, err := otel.InitTracer("aether-node")
	if err != nil {
		panic(fmt.Sprintf("无法初始化 OpenTelemetry: %v", err) )
	}
	defer shutdown(context.Background())

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法加载配置: %v\n", err)
		os.Exit(1)
	}

	modeFlag := flag.String("mode", "", "运行模式")
	configPath := flag.String("config", "", "配置文件路径")
	flag.Parse()

	// 如果指定了配置文件，重新加载
	if *configPath != "" {
		newCfg, err := config.Load(*configPath)
		if err == nil {
			cfg = newCfg
		}
	}

	if *modeFlag != "" { cfg.App.Mode = *modeFlag }

	rt := app.NewDefaultRuntime(cfg)
	defer rt.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.App.Mode {
	case "single":
		handleSingle(ctx, rt, flag.Args())
	case "cluster-leader":
		startLeader(ctx, rt, "")
	case "cluster-worker":
		startWorker(ctx, rt, cfg.App.Role, cfg.App.NodeID)
	}
}

func handleSingle(ctx context.Context, rt *app.Runtime, args []string) {
	if len(args) < 1 {
		printUsage()
		return
	}

	switch args[0] {
	case "strategic":
		handleStrategic(ctx, rt, args[1:])
	case "knowledge":
		handleKnowledge(ctx, rt, args[1:])
	case "export":
		handleExport(ctx, rt, args[1:])
	case "run":
		handleTask(ctx, rt, args[1:])
	case "task":
		handleTask(ctx, rt, args[1:])
	case "skill":
		cli.NewSkillHandler(rt).Handle(ctx, args[1:])
	case "pipeline":
		cli.NewPipelineHandler(rt).Handle(ctx, args[1:])
	default:
		printUsage()
	}
}

func handleKnowledge(ctx context.Context, rt *app.Runtime, args []string) {
	if len(args) < 1 {
		fmt.Println("用法: aether knowledge <query|relations> [arguments]")
		return
	}

	orgID := "default" // 简化演示

	switch args[0] {
	case "query":
		queryCmd := flag.NewFlagSet("query", flag.ExitOnError)
		entityType := queryCmd.String("type", "", "实体类型")
		queryCmd.Parse(args[1:])
		results, _ := rt.KnowledgeGraph().QueryByType(ctx, orgID, *entityType)
		printJSON(results)
	case "relations":
		relCmd := flag.NewFlagSet("relations", flag.ExitOnError)
		id := relCmd.String("id", "", "实体 ID")
		relCmd.Parse(args[1:])
		results, _ := rt.KnowledgeGraph().GetRelations(ctx, orgID, *id)
		printJSON(results)
	}
}

func handleExport(ctx context.Context, rt *app.Runtime, args []string) {
	if len(args) < 1 {
		fmt.Println("用法: aether export <audit|ledger|proposals> --org=<id>")
		return
	}

	exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
	orgID := exportCmd.String("org", "default", "组织 ID")
	exportCmd.Parse(args[1:])

	var data any
	var err error

	switch args[0] {
	case "audit":
		data, err = rt.Audit().QueryByTimeRange(ctx, *orgID, time.Now().Add(-720*time.Hour), time.Now())
	case "ledger":
		data, err = rt.Ledger().ListTransactions(ctx, *orgID)
	case "proposals":
		data = rt.Governance().ListProposals(*orgID)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
		return
	}
	printJSON(data)
}

func handleStrategic(ctx context.Context, rt *app.Runtime, args []string) {
	if len(args) < 1 { return }
	switch args[0] {
	case "vision":
		vCmd := flag.NewFlagSet("vision", flag.ExitOnError)
		title := vCmd.String("title", "", "标题")
		desc := vCmd.String("desc", "", "描述")
		vCmd.Parse(args[1:])
		v, _ := rt.StrategicPlanner().CreateVision(ctx, *title, *desc)
		rt.StrategicStore().SaveVision(v)
		goals, _ := rt.StrategicPlanner().PlanGoals(ctx, *v)
		rt.StrategicStore().SaveGoals(goals)
		fmt.Printf("Vision created: %s\n", v.ID)
	case "goal":
		goals, _ := rt.StrategicStore().ListActiveGoals()
		printJSON(goals)
	case "start":
		fmt.Println("Starting Strategic Engine...")
		go rt.StrategicEngine().Start(ctx)
		rt.InitAgent("supervisor")
		rt.InitAgent("planner")
		rt.InitAgent("coder")
		rt.InitAgent("reviewer")
		rt.StartAgents(ctx)
		<-ctx.Done()
	}
}

func printJSON(data any) {
	b, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(b))
}

func handleTask(ctx context.Context, rt *app.Runtime, args []string) {
	if len(args) < 1 {
		fmt.Println("用法: aether task <task_description>")
		return
	}
	taskDesc := args[0]

	// 1. Tracing: 开启根 Span
	tracer := go_otel.Tracer("aether-cli")
	ctx, span := tracer.Start(ctx, "cli.task_execution")
	span.SetAttributes(attribute.String("task.description", taskDesc))
	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	fmt.Printf("\n🚀 启动 Aether 协作任务 (TraceID: %s)\n", traceID)
	fmt.Printf("🔗 Jaeger 监控: http://localhost:16686/trace/%s\n", traceID)
	fmt.Println("--------------------------------------------------------------------------------")

	// 2. 核心：在启动 Agent 前先订阅。统一反馈主题为 "cli"
	doneChan := make(chan string, 1)
	rt.GetBus().SubscribeToSubject(ctx, "cli", func(msg agent.Message) {
		switch msg.Type {
		case "token":
			if token, ok := msg.Payload["token"].(string); ok {
				agentName, _ := msg.Payload["agent"].(string)
				color := "\033[37m" 
				if strings.Contains(agentName, "planner") { color = "\033[32m" }
				if strings.Contains(agentName, "supervisor") { color = "\033[35m" }
				if strings.Contains(agentName, "coder") { color = "\033[34m" }
				if strings.Contains(agentName, "reviewer") { color = "\033[33m" }

				processedToken := token
				if strings.Contains(token, "Thought:") { processedToken = "\033[1;33m" + token + "\033[0m" + color }
				if strings.Contains(token, "Action:") { processedToken = "\033[1;32m" + token + "\033[0m" + color }
				if strings.Contains(token, "Observation:") { processedToken = "\033[1;36m" + token + "\033[0m" + color }

				fmt.Fprintf(os.Stderr, "%s%s\033[0m", color, processedToken)
			}
		case "final_report":
			result, _ := msg.Payload["result"].(string)
			doneChan <- result
		case "system.healing":
			fmt.Printf("\n\033[1;31m🛠️  [自愈系统] %v\033[0m\n", msg.Payload["message"])
		}
	})

	// 3. 唤醒集群
	go rt.StartAgents(ctx)
	fmt.Print("⚙️  系统正在冷启动模型与订阅总线...")
	time.Sleep(1 * time.Second)
	fmt.Println(" [OK]")

	// 4. 下发任务
	taskID := fmt.Sprintf("t-%d", time.Now().Unix())
	fmt.Fprintf(os.Stderr, "📡 任务下发 (ID: %s)...\n", taskID)
	fmt.Println("🧠 Thinking...")
	
	rt.GetBus().Publish(ctx, agent.Message{
		ID: taskID, From: "cli", To: "supervisor",
		Type: "task", Timestamp: time.Now(),
		Payload: map[string]any{
			"description": taskDesc, 
			"org_id": "default",
			"trace_id": traceID,
		},
	})

	// 5. 阻塞等待直至完成或超时
	select {
	case result := <-doneChan:
		fmt.Printf("\n--------------------------------------------------------------------------------")
		fmt.Printf("\n✨ 任务执行成功!\n\n--- 最终交付物 ---\n%s\n------------------\n", result)
		fmt.Println("\n✅ Aether 流程已全线闭环。")
		time.Sleep(500 * time.Millisecond) // 留时间给 OTel Flush
		os.Exit(0) // 强制退出
	case <-ctx.Done():
		fmt.Println("\n🛑 用户手动中断或执行超时")
		os.Exit(1)
	}
}

func startLeader(ctx context.Context, rt *app.Runtime, task string) {
	la := &leaderAgent{BaseAgent: *agent.NewBaseAgent("leader", "system-leader"), scheduler: cluster.NewScheduler(rt.Logger(), rt.Ledger(), nil)} // Guard 为 nil
	rt.GetBus().Subscribe(la)
	rt.StartAgents(ctx)
	<-ctx.Done()
}

func startWorker(ctx context.Context, rt *app.Runtime, role, nodeID string) {
	if err := rt.InitAgent(role); err != nil { return }
	cluster.StartWorkerHeartbeat(ctx, rt.GetBus(), role, nodeID, rt.Logger())
	rt.StartAgents(ctx)
	<-ctx.Done()
}

type leaderAgent struct {
	agent.BaseAgent
	scheduler *cluster.Scheduler
}

func (l *leaderAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	if msg.Type == "heartbeat" {
		role, _ := msg.Payload["role"].(string)
		workerID, _ := msg.Payload["worker_id"].(string)
		l.scheduler.RegisterHeartbeat(role, workerID)
	}
	return nil, nil
}

func printUsage() {
	fmt.Println("AetherCLI - 企业级 AI 操作系统")
}

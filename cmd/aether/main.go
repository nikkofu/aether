package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/internal/delivery/cli"
	"github.com/nikkofu/aether/internal/usecase/cluster"
	"github.com/nikkofu/aether/pkg/config"
	"github.com/nikkofu/aether/pkg/observability/otel"
	"github.com/nikkofu/aether/internal/app"
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
		handleRun(rt, args[1:])
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

func handleRun(rt *app.Runtime, args []string) {
	if len(args) < 1 {
		fmt.Println("用法: aether run <task_description>")
		return
	}
	taskDesc := args[0]

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("🚀 启动 Agentic 协作任务: %s\n", taskDesc)
	fmt.Println("--------------------------------------------------------------------------------")

	// 1. 初始化并启动系统级代理
	rt.StartAgents(ctx)
	
	// 给异步订阅一点启动时间
	time.Sleep(200 * time.Millisecond)

	// 2. 向总线发布任务指令
	rt.GetBus().Publish(ctx, agent.Message{
		ID:        "task-" + time.Now().Format("150405"),
		From:      "cli",
		To:        "supervisor",
		Type:      "task",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"description": taskDesc,
		},
	})

	// 3. 阻塞直到任务完成或手动退出
	// 这里的简化逻辑是等待上下文结束，实际场景中可以通过监听 final_report 结束
	<-ctx.Done()
	fmt.Println("\n--------------------------------------------------------------------------------")
	fmt.Println("✅ 任务流执行完毕")
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

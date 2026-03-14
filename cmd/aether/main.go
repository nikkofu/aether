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

	"github.com/nikkofu/aether/internal/app"
	"github.com/nikkofu/aether/internal/delivery/cli"
	"github.com/nikkofu/aether/internal/domain/agent"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	"github.com/nikkofu/aether/internal/usecase/cluster"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
	"github.com/nikkofu/aether/pkg/config"
	"github.com/nikkofu/aether/pkg/observability/otel"
	go_otel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func main() {
	// 初始化 OpenTelemetry
	shutdown, err := otel.InitTracer("aether-node")
	if err != nil {
		panic(fmt.Sprintf("无法初始化 OpenTelemetry: %v", err))
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

	if *modeFlag != "" {
		cfg.App.Mode = *modeFlag
	}

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
	if len(args) < 1 {
		return
	}
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
		rt.StartAgentsForPatterns(ctx, taskdomain.PatternSequential, taskdomain.PatternParallel, taskdomain.PatternCoordinator, taskdomain.PatternHierarchical)
		<-ctx.Done()
	}
}

func printJSON(data any) {
	b, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(b))
}

func handleTask(ctx context.Context, rt *app.Runtime, args []string) {
	taskCmd := flag.NewFlagSet("task", flag.ContinueOnError)
	workflowFlag := taskCmd.String("workflow", string(taskdomain.PatternSequential), "执行工作流模式")
	maxReviewIterations := taskCmd.Int("max-review-iterations", 3, "loop / review_critique / iterative_refinement 模式下的最大评审轮次")
	parallelBranches := taskCmd.String("parallel-branches", "", "parallel 模式下的分支列表，使用 '||' 或换行分隔")
	if err := taskCmd.Parse(args); err != nil {
		fmt.Println("用法: aether task [-workflow=sequential|parallel|loop|coordinator|hierarchical|iterative_refinement|review_critique] [-max-review-iterations=3] [-parallel-branches='branch1||branch2'] <task_description>")
		return
	}
	if taskCmd.NArg() < 1 {
		fmt.Println("用法: aether task [-workflow=sequential|parallel|loop|coordinator|hierarchical|iterative_refinement|review_critique] [-max-review-iterations=3] [-parallel-branches='branch1||branch2'] <task_description>")
		return
	}
	taskDesc := parseTaskDescriptionArgs(taskCmd.Args())
	if taskDesc == "" {
		fmt.Println("用法: aether task [-workflow=sequential|parallel|loop|coordinator|hierarchical|iterative_refinement|review_critique] [-max-review-iterations=3] [-parallel-branches='branch1||branch2'] <task_description>")
		return
	}
	workflowPattern := taskdomain.WorkflowPattern(*workflowFlag)
	taskService := rt.TaskService()
	if taskService == nil {
		fmt.Fprintln(os.Stderr, "任务服务不可用，无法提交任务。")
		os.Exit(1)
	}

	// 1. Tracing: 开启根 Span
	tracer := go_otel.Tracer("aether-cli")
	ctx, span := tracer.Start(ctx, "cli.task_execution")
	span.SetAttributes(attribute.String("task.description", taskDesc))
	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	fmt.Printf("\n🚀 启动 Aether 协作任务 (TraceID: %s)\n", traceID)
	fmt.Printf("🔗 Jaeger 监控: http://localhost:16686/trace/%s\n", traceID)
	fmt.Println("--------------------------------------------------------------------------------")

	// 2. 核心：订阅 CLI 主题获取流式 token，再通过 TaskService 观察任务生命周期。
	doneChan := make(chan *taskdomain.Task, 1)
	rt.GetBus().SubscribeToSubject(ctx, "cli", func(msg agent.Message) {
		switch msg.Type {
		case "token":
			if token, ok := msg.Payload["token"].(string); ok {
				agentName, _ := msg.Payload["agent"].(string)
				color := "\033[37m"
				if strings.Contains(agentName, "planner") {
					color = "\033[32m"
				}
				if strings.Contains(agentName, "supervisor") {
					color = "\033[35m"
				}
				if strings.Contains(agentName, "coder") {
					color = "\033[34m"
				}
				if strings.Contains(agentName, "reviewer") {
					color = "\033[33m"
				}

				processedToken := token
				if strings.Contains(token, "Thought:") {
					processedToken = "\033[1;33m" + token + "\033[0m" + color
				}
				if strings.Contains(token, "Action:") {
					processedToken = "\033[1;32m" + token + "\033[0m" + color
				}
				if strings.Contains(token, "Observation:") {
					processedToken = "\033[1;36m" + token + "\033[0m" + color
				}

				fmt.Fprintf(os.Stderr, "%s%s\033[0m", color, processedToken)
			}
		case "system.healing":
			fmt.Printf("\n\033[1;31m🛠️  [自愈系统] %v\033[0m\n", msg.Payload["message"])
		}
	})

	// 3. 唤醒集群
	go rt.StartAgentsForPatterns(ctx, workflowPattern)
	fmt.Print("⚙️  系统正在冷启动模型与订阅总线...")
	time.Sleep(1 * time.Second)
	fmt.Println(" [OK]")

	// 4. 统一通过 TaskService 下发任务。
	var taskInput map[string]any
	switch taskdomain.NormalizeWorkflowPattern(workflowPattern) {
	case taskdomain.PatternParallel:
		if branches := parseParallelBranchesFlag(*parallelBranches); len(branches) > 0 {
			taskInput = map[string]any{
				taskdomain.ParallelBranchesInputKey: branches,
			}
		}
	case taskdomain.PatternLoop, taskdomain.PatternReviewCritique, taskdomain.PatternIterativeRefinement:
		taskInput = map[string]any{
			taskdomain.MaxReviewIterationsInputKey: *maxReviewIterations,
		}
	}

	submittedTask, err := taskService.Submit(ctx, taskusecase.SubmitInput{
		Source:          "cli",
		Mode:            "agent",
		WorkflowPattern: workflowPattern,
		Description:     taskDesc,
		Input:           taskInput,
		TraceID:         traceID,
		OrgID:           "default",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "任务提交失败: %v\n", err)
		os.Exit(1)
	}

	taskID := submittedTask.ID
	updates, cancel := taskService.Subscribe(taskID, 64)
	defer cancel()

	go func() {
		for update := range updates {
			if update.Event != nil && shouldLogTaskEvent(update.Event.Type) {
				from := update.Event.From
				if from == "" {
					from = "task-service"
				}
				to := update.Event.To
				if to == "" {
					to = "-"
				}
				fmt.Fprintf(os.Stderr, "\n[%s] %s -> %s\n", update.Event.Type, from, to)
			}

			if update.Task != nil && isTerminalTaskStatus(update.Task.Status) {
				select {
				case doneChan <- update.Task:
				default:
				}
				return
			}
		}
	}()

	currentTask, err := taskService.Get(ctx, taskID)
	if err == nil && isTerminalTaskStatus(currentTask.Status) {
		select {
		case doneChan <- currentTask:
		default:
		}
	}

	fmt.Fprintf(os.Stderr, "📡 任务下发 (ID: %s, workflow: %s)...\n", taskID, submittedTask.WorkflowPattern)
	fmt.Println("🧠 Thinking...")

	// 5. 阻塞等待直至完成或超时
	select {
	case result := <-doneChan:
		if result.Status != taskdomain.StatusCompleted {
			fmt.Printf("\n--------------------------------------------------------------------------------")
			fmt.Printf("\n❌ 任务未完成 (%s)\n", result.Status)
			if result.ErrorSummary != "" {
				fmt.Printf("\n--- 错误摘要 ---\n%s\n----------------\n", result.ErrorSummary)
			}
			time.Sleep(500 * time.Millisecond)
			os.Exit(1)
		}
		fmt.Printf("\n--------------------------------------------------------------------------------")
		fmt.Printf("\n✨ 任务执行成功!\n\n--- 最终交付物 ---\n%s\n------------------\n", result.FinalOutput)
		fmt.Println("\n✅ Aether 流程已全线闭环。")
		time.Sleep(500 * time.Millisecond) // 留时间给 OTel Flush
		os.Exit(0)                         // 强制退出
	case <-ctx.Done():
		fmt.Println("\n🛑 用户手动中断或执行超时")
		os.Exit(1)
	}
}

func isTerminalTaskStatus(status taskdomain.Status) bool {
	switch status {
	case taskdomain.StatusCompleted,
		taskdomain.StatusFailed,
		taskdomain.StatusCancelled,
		taskdomain.StatusInterrupted:
		return true
	default:
		return false
	}
}

func shouldLogTaskEvent(eventType string) bool {
	switch eventType {
	case "task.submitted",
		agent.TypeWorkflowCoordinatorStart,
		agent.TypeWorkflowHierarchicalStart,
		agent.TypeWorkflowIterativeStart,
		agent.TypeWorkflowLoopStart,
		agent.TypeWorkflowParallelStart,
		agent.TypeWorkflowSequentialStart,
		agent.TypeWorkflowReviewCritiqueStart,
		"goal.assigned",
		agent.TypeGoalResult,
		"milestone.assigned",
		"milestone.feedback",
		"task.assigned",
		"task.completed",
		agent.TypeCoordinationResult,
		"task_plan_request",
		agent.TypePlanGenerated,
		"instruction",
		agent.TypeDraftGenerated,
		"review_request",
		"review_result",
		"final_report",
		"task.dispatch_failed",
		agent.TypeSystemAlert:
		return true
	default:
		return false
	}
}

func startLeader(ctx context.Context, rt *app.Runtime, task string) {
	la := &leaderAgent{BaseAgent: *agent.NewBaseAgent("leader", "system-leader"), scheduler: cluster.NewScheduler(rt.Logger(), rt.Ledger(), nil)} // Guard 为 nil
	rt.GetBus().Subscribe(la)
	rt.StartAgents(ctx)
	<-ctx.Done()
}

func startWorker(ctx context.Context, rt *app.Runtime, role, nodeID string) {
	if err := rt.InitAgent(role); err != nil {
		return
	}
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

func parseParallelBranchesFlag(raw string) []map[string]any {
	return taskdomain.ParallelBranchesToInput(taskdomain.ParseParallelBranchesText(raw))
}

func parseTaskDescriptionArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(args, " "))
}

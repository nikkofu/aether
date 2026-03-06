package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/nikkofu/aether/internal/domain/agent"
	"github.com/nikkofu/aether/pkg/audit"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/capability/skills"
	"github.com/nikkofu/aether/internal/infrastructure/capabilities"
	cap_alarm "github.com/nikkofu/aether/internal/infrastructure/capabilities/alarm"
	cap_calendar "github.com/nikkofu/aether/internal/infrastructure/capabilities/calendar"
	cap_code "github.com/nikkofu/aether/internal/infrastructure/capabilities/code"
	cap_diary "github.com/nikkofu/aether/internal/infrastructure/capabilities/diary"
	cap_expense "github.com/nikkofu/aether/internal/infrastructure/capabilities/expense"
	cap_file "github.com/nikkofu/aether/internal/infrastructure/capabilities/file"
	cap_gateway "github.com/nikkofu/aether/internal/infrastructure/capabilities/gateway"
	cap_reg "github.com/nikkofu/aether/internal/infrastructure/capabilities/registry"
	cap_search "github.com/nikkofu/aether/internal/infrastructure/capabilities/search"
	"github.com/nikkofu/aether/internal/infrastructure/llm"
	"github.com/nikkofu/aether/internal/infrastructure/llm/ollama"
	"github.com/nikkofu/aether/internal/infrastructure/llm/openai"
	"github.com/nikkofu/aether/internal/usecase/cluster"
	"github.com/nikkofu/aether/pkg/config"
	"github.com/nikkofu/aether/internal/usecase/dag"
	"github.com/nikkofu/aether/internal/domain/economy"
	"github.com/nikkofu/aether/internal/domain/governance"
	"github.com/nikkofu/aether/internal/domain/governance/constitution"
	"github.com/nikkofu/aether/internal/domain/issue"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/internal/usecase/learning"
	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/internal/core/memory"
	"github.com/nikkofu/aether/pkg/metrics"
	"github.com/nikkofu/aether/pkg/observability"
	"github.com/nikkofu/aether/pkg/observability/otel"
	"github.com/nikkofu/aether/pkg/observability/trace"
	"github.com/nikkofu/aether/internal/domain/org"
	"github.com/nikkofu/aether/internal/domain/policy"
	"github.com/nikkofu/aether/internal/usecase/reflection"
	"github.com/nikkofu/aether/internal/domain/risk"
	"github.com/nikkofu/aether/pkg/routing"
	"github.com/nikkofu/aether/pkg/security/rbac"
	"github.com/nikkofu/aether/internal/usecase/skills/engine"
	skill_registry "github.com/nikkofu/aether/internal/usecase/skills/registry"
	skill_sandbox "github.com/nikkofu/aether/internal/usecase/skills/sandbox"
	"github.com/nikkofu/aether/internal/domain/strategy"
	"github.com/nikkofu/aether/internal/domain/strategy/evolution"
	"github.com/nikkofu/aether/internal/domain/strategy/strategic"
	sys_scheduler "github.com/nikkofu/aether/internal/usecase/system/scheduler"
)

// Runtime 是 Aether 系统的核心运行时。
type Runtime struct {
	cfg             *config.Config
	db              *sql.DB
	adapterMap      map[string]llm.Adapter
	registry        *capability.CapabilityRegistry
	policy          policy.Policy
	memory          memory.Store
	tracker         metrics.Tracker
	router          *routing.DefaultRouter
	tracer          observability.Tracer
	logger          logging.Logger
	bus             bus.Bus
	agentManager    agent.AgentManager
	issueHandler    issue.Handler
	reflector       reflection.Reflector
	reflectionStore reflection.Store
	learningEngine  *learning.LearningEngine
	strategyStore   strategy.StrategyStore
	strategicPlanner strategic.StrategicPlanner
	strategicStore   strategic.Store
	strategicEngine  *strategic.Engine
	orgRegistry     org.OrgRegistry
	knowledgeGraph  knowledge.Graph
	ledger          economy.Ledger
	governanceBoard *governance.GovernanceBoard
	constitution    constitution.Constitution
	sysScheduler    *sys_scheduler.SystemScheduler
	rbac            rbac.RBAC
	audit           audit.Logger
	riskGuard       *risk.RiskGuard
	govLock         *governance.GovernanceLock
	
	evolutionGuard  *policy.EvolutionGuard
	strategyEngine  evolution.StrategyEngine

	// 追踪引擎
	traceEngine     *trace.TraceEngine

	// 调度器
	scheduler       *cluster.Scheduler

	// 能力网关
	capabilityGateway capabilities.Gateway
	wasmExecutor      *skill_sandbox.WASMExecutor
	compiler          *engine.PolyglotCompiler
}

func (r *Runtime) GetAdapter(name string) (llm.Adapter, bool) {
	a, ok := r.adapterMap[name]
	return a, ok
}

func (r *Runtime) Ledger() economy.Ledger { return r.ledger }
func (r *Runtime) Audit() audit.Logger    { return r.audit }
func (r *Runtime) Memory() memory.Store   { return r.memory }
func (r *Runtime) Governance() *governance.GovernanceBoard { return r.governanceBoard }
func (r *Runtime) KnowledgeGraph() knowledge.Graph { return r.knowledgeGraph }
func (r *Runtime) StrategicPlanner() strategic.StrategicPlanner { return r.strategicPlanner }
func (r *Runtime) StrategicStore() strategic.Store { return r.strategicStore }
func (r *Runtime) StrategicEngine() *strategic.Engine { return r.strategicEngine }
func (r *Runtime) CapabilityGateway() capabilities.Gateway { return r.capabilityGateway }
func (r *Runtime) WASMExecutor() *skill_sandbox.WASMExecutor { return r.wasmExecutor }
func (r *Runtime) Compiler() *engine.PolyglotCompiler { return r.compiler }
func (r *Runtime) Scheduler() *cluster.Scheduler { return r.scheduler }

func NewRuntime(cfg *config.Config) *Runtime {
	t := observability.NewConsoleRenderer()
	logFormat := "console"
	if os.Getenv("AETHER_LOG_FORMAT") == "json" { logFormat = "json" }
	l, _ := logging.NewLogger(logging.Config{Level: cfg.Log.Level, Format: logFormat})

	var b bus.Bus
	if cfg.Runtime.NATSURL != "" {
		b, _ = bus.NewNATSBus(cfg.Runtime.NATSURL)
	} else {
		b = bus.NewMemoryBus(100)
	}

	return &Runtime{
		cfg:        cfg,
		adapterMap: make(map[string]llm.Adapter),
		registry:   capability.NewCapabilityRegistry(),
		policy:     policy.NewDefaultPolicy(),
		memory:     memory.NewInMemoryStore(),
		router:     routing.NewDefaultRouter(nil, t),
		tracer:     t,
		logger:     l,
		bus:        b,
		orgRegistry: org.NewInMemoryRegistry(),
		govLock:    governance.NewGovernanceLock(),
		evolutionGuard: policy.NewEvolutionGuard(),
	}
}

func NewDefaultRuntime(cfg *config.Config) *Runtime {
	// 初始化 OTEL (Jaeger)
	if _, err := otel.InitTracer("aether-core"); err != nil {
		fmt.Printf("警告: 无法初始化 OTEL 追踪器: %v\n", err)
	}

	r := NewRuntime(cfg)
	if db, err := sql.Open("sqlite", cfg.Runtime.DatabasePath); err == nil {
		db.SetMaxOpenConns(1)
		r.db = db

		// 初始化 Trace 存储和引擎
		traceStorage, _ := trace.NewSQLiteTraceStorage(cfg.Runtime.DatabasePath)
		r.traceEngine = trace.NewTraceEngine(traceStorage)

		r.memory, _ = memory.NewSQLiteStoreWithDB(db)
		r.tracker, _ = metrics.NewSQLiteTracker(db)
		r.issueHandler, _ = issue.NewSQLiteHandler(db, r.logger)
		r.reflectionStore, _ = reflection.NewSQLiteStore(db)
		r.strategyStore, _ = strategy.NewSQLiteStrategyStore(db)
		r.learningEngine = learning.NewLearningEngine(r.strategyStore)
		r.strategicStore, _ = strategic.NewSQLiteStore(db)
		r.knowledgeGraph, _ = knowledge.NewSQLiteGraph(db)
		r.ledger, _ = economy.NewSQLiteLedger(db)
		r.constitution, _ = constitution.NewSQLiteConstitution(db)
		r.audit, _ = audit.NewSQLiteLogger(db)
		r.rbac, _ = rbac.NewSQLiteRBAC(db)
		
		// 1. 初始化能力网关
		capReg := cap_reg.NewRegistry()
		limiter := cap_gateway.NewRateLimiter(100, 500)
		r.capabilityGateway = cap_gateway.NewDefaultGateway(capReg, r.rbac, r.audit, limiter, r.traceEngine)

		// 2. 初始化 WASM 执行器 (注入网关)
		r.wasmExecutor, _ = skill_sandbox.NewWASMExecutor(context.Background(), r.capabilityGateway, r.traceEngine)

		// 3. 注册原子能力
		r.capabilityGateway.Register(cap_file.NewFileCapability("./data"))
		r.capabilityGateway.Register(cap_search.NewSearchCapability())
		r.capabilityGateway.Register(cap_code.NewCodeCapability())
		
		cal, _ := cap_calendar.NewCalendarCapability(db)
		r.capabilityGateway.Register(cal)
		
		exp, _ := cap_expense.NewExpenseCapability(db)
		r.capabilityGateway.Register(exp)
		
		dry, _ := cap_diary.NewDiaryCapability(db)
		r.capabilityGateway.Register(dry)
		
		alm, _ := cap_alarm.NewAlarmCapability(db)
		r.capabilityGateway.Register(alm)

		r.governanceBoard = governance.NewGovernanceBoard(r.ledger, r.constitution, r.policy, r.rbac, r.audit, r.govLock, r.logger)
		r.sysScheduler = sys_scheduler.NewSystemScheduler(r.ledger, r.logger)
		r.riskGuard = risk.NewRiskGuard(r.ledger, 100000.0, 0.4, 0.2)

		// 初始化调度器
		r.scheduler = cluster.NewScheduler(r.logger, r.ledger, r.riskGuard)
		if cfg.App.Mode == "cluster-leader" {
			r.scheduler.Start(context.Background(), r.bus)
		}
	}

	// 初始化 LLM (默认优先使用 Ollama)
	llmSkill := r.initLLM(cfg)

	r.strategyEngine = evolution.NewDefaultStrategyEngine(llmSkill, r.knowledgeGraph, r.logger, r.evolutionGuard)
	r.reflector = reflection.NewLLMReflector(llmSkill)
	r.strategicPlanner = strategic.NewLLMStrategicPlanner(llmSkill, r.knowledgeGraph, r.strategyEngine, r.logger)

	skillEngine, _ := skill_registry.NewSQLiteSkillEngine(r.db, r.wasmExecutor, "./data/wasm_cache")
	r.compiler = engine.NewPolyglotCompiler(skillEngine, "llm", r.logger, "./data/wasm_cache")

	dm := agent.NewDefaultAgentManager(llmSkill, r.tracer, r.logger, r.bus, r.knowledgeGraph, cfg.Agent.MaxSpawnPerTask, cfg.Agent.MaxConcurrency, r.traceEngine)
	dm.SetLearning(r.reflector, r.reflectionStore, r.learningEngine)
	dm.SetScheduler(r.scheduler)
	dm.RegisterRole("operational", func(ctx context.Context, name string, payload map[string]any) (agent.Agent, error) {

		return org.NewOperationalWorkerAgent(name, "tactical_manager", llmSkill, r.reflector, r.ledger, r.wasmExecutor, r.traceEngine), nil
	})
	r.agentManager = dm
	r.strategicEngine = strategic.NewEngine(r.strategicPlanner, r.strategicStore, r.agentManager, r.bus, r.logger, r.traceEngine)

	return r
}

func (r *Runtime) initSystemAgents() {
	supervisor := agent.NewSupervisorAgent("supervisor", r.tracer, r.logger)
	supervisor.SetGraph(r.knowledgeGraph)
	r.agentManager.Register(supervisor)
	r.bus.Subscribe(supervisor)

	// 预注册核心协作团队
	llm, _ := r.registry.Get("llm")
	
	planner := agent.NewPlannerAgent("planner", llm, r.tracer)
	planner.SetGraph(r.knowledgeGraph)
	r.agentManager.Register(planner)
	r.bus.Subscribe(planner)

	coder := agent.NewCoderAgent("coder", llm, r.tracer)
	r.agentManager.Register(coder)
	r.bus.Subscribe(coder)

	reviewer := agent.NewReviewerAgent("reviewer", llm, r.tracer)
	r.agentManager.Register(reviewer)
	r.bus.Subscribe(reviewer)

	sentinel := agent.NewSentinelAgent("sentinel", agent.SentinelConfig{MaxDurationThreshold: 30 * time.Second, CostBudget: 1.0}, r.logger)
	r.agentManager.Register(sentinel)
	r.bus.Subscribe(sentinel)

	issueWatcher := &issueWatcherAgent{BaseAgent: *agent.NewBaseAgent("issue_handler", "system-watcher"), handler: r.issueHandler, graph: r.knowledgeGraph}
	r.bus.Subscribe(issueWatcher)
}

type issueWatcherAgent struct {
	agent.BaseAgent
	handler issue.Handler
	graph   knowledge.Graph
}

func (a *issueWatcherAgent) Handle(ctx context.Context, msg agent.Message) ([]agent.Message, error) {
	if msg.Type == "system.alert" {
		orgID := "default"
		if m, ok := msg.Payload["org_id"].(string); ok { orgID = m }
		iss := issue.Issue{ID: msg.ID, Severity: "HIGH", Source: msg.From, Message: msg.Payload["message"].(string), CreatedAt: time.Now()}
		a.handler.Report(ctx, iss)
		if a.graph != nil {
			a.graph.AddEntity(ctx, knowledge.Entity{ID: iss.ID, Type: "issue", Name: "Alert:" + msg.From}, orgID)
			a.graph.AddRelation(ctx, knowledge.Relation{ID: "rel-"+iss.ID, FromID: msg.From, ToID: iss.ID, Type: "raised_issue"}, orgID)
		}
	}
	return nil, nil
}

func (r *Runtime) InitAgent(role string) error {
	llm, _ := r.registry.Get("llm")
	var a agent.Agent
	switch role {
	case "supervisor": a = agent.NewSupervisorAgent(role, r.tracer, r.logger)
	case "planner": a = agent.NewPlannerAgent(role, llm, r.tracer)
	case "coder": a = agent.NewCoderAgent(role, llm, r.tracer)
	case "reviewer": a = agent.NewReviewerAgent(role, llm, r.tracer)
	default: return fmt.Errorf("unknown role: %s", role)
	}
	if ba, ok := a.(interface {
		SetComponents(reflection.Reflector, reflection.Store, *learning.LearningEngine, logging.Logger)
	}); ok {
		ba.SetComponents(r.reflector, r.reflectionStore, r.learningEngine, r.logger)
	}
	r.agentManager.Register(a)
	r.bus.Subscribe(a)
	return nil
}

func (r *Runtime) RegisterAdapter(a llm.Adapter) {
	if a == nil { return }
	r.adapterMap[a.Name()] = a
	names := make([]string, 0, len(r.adapterMap))
	for n := range r.adapterMap { names = append(names, n) }
	r.router.UpdateAdapters(names)
}

func (r *Runtime) StartAgents(ctx context.Context) {
	fmt.Fprintf(os.Stderr, "\033[1;35m⚙️  正在唤醒智能体集群...\033[0m")
	
	// 1. 系统角色同步注册
	r.initSystemAgents()
	r.initOrgAgents()

	if r.sysScheduler != nil {
		go r.sysScheduler.Start(ctx)
	}

	// 2. 异步启动总线消费循环
	go r.bus.Start(ctx)
	
	fmt.Fprintf(os.Stderr, " \033[1;32m[OK]\033[0m\n\033[1;32m🚀 集群已完全就绪，开始处理任务。\033[0m\n")
}
func (r *Runtime) GetBus() bus.Bus                          { return r.bus }
func (r *Runtime) AgentManager() agent.AgentManager         { return r.agentManager }
func (r *Runtime) Logger() logging.Logger                   { return r.logger }
func (r *Runtime) Config() *config.Config                   { return r.cfg }
func (r *Runtime) Execute(ctx context.Context, adapter, prompt string) (string, error) {
	a, ok := r.adapterMap[adapter]; if !ok { return "", fmt.Errorf("adapter not found") }
	return a.Execute(ctx, prompt)
}
func (r *Runtime) NewPipelineExecutor(workers int) *dag.PipelineExecutor {
	return dag.NewPipelineExecutor(r.registry, r.policy, r.memory, r.tracer, r.logger, workers)
}
func (r *Runtime) GetRegistry() *capability.CapabilityRegistry { return r.registry }
func (r *Runtime) Close() error {
	if r.db != nil { return r.db.Close() }
	return nil
}

var _ skills.AdapterProvider = (*Runtime)(nil)

func (r *Runtime) initOrgAgents() {
	vb := org.NewVisionBoardAgent("vision_board", r.strategicPlanner, r.logger)
	r.injectAndRegisterOrg(vb)
	sd := org.NewStrategicDirectorAgent("strategic_director", r.strategicPlanner, r.logger)
	r.injectAndRegisterOrg(sd)
	llm, _ := r.registry.Get("llm")
	tm := org.NewTacticalManagerAgent("tactical_manager", "strategic_director", r.agentManager, llm, r.tracer)
	r.injectAndRegisterOrg(tm)
	gv := org.NewGovernanceAgent("governance")
	r.injectAndRegisterOrg(gv)
}

func (r *Runtime) injectAndRegisterOrg(a org.OrgAgent) {
	if ba, ok := a.(interface {
		SetComponents(reflection.Reflector, reflection.Store, *learning.LearningEngine, logging.Logger)
	}); ok {
		ba.SetComponents(r.reflector, r.reflectionStore, r.learningEngine, r.logger)
	}
	r.orgRegistry.Register(a)
	r.agentManager.Register(a)
	r.bus.Subscribe(a)
}

func (r *Runtime) initLLM(cfg *config.Config) capability.Capability {
	// 1. 注册 Ollama (优先，作为本地默认)
	if cfg.Ollama.BaseURL != "" {
		ola := ollama.NewOllamaAdapter(ollama.Config{
			BaseURL: cfg.Ollama.BaseURL, Model: cfg.Ollama.Model,
			Temperature: cfg.Ollama.Temperature, Timeout: cfg.Ollama.Timeout,
		})
		r.RegisterAdapter(ola)
	}

	// 2. 注册 OpenAI (可选)
	if cfg.OpenAI.APIKey != "" {
		oa := openai.NewOpenAIAdapter(openai.Config{
			BaseURL: cfg.OpenAI.BaseURL, APIKey: cfg.OpenAI.APIKey, Model: cfg.OpenAI.Model,
			Temperature: cfg.OpenAI.Temperature, Timeout: cfg.OpenAI.Timeout,
		})
		if r.traceEngine != nil {
			oa.SetTracer(r.tracer)
			oa.SetTraceEngine(r.traceEngine)
		}
		r.RegisterAdapter(oa)
	}

	// 3. 选择默认适配器: Ollama > OpenAI
	var defaultAdapter llm.Adapter
	if a, ok := r.adapterMap["ollama"]; ok {
		defaultAdapter = a
	} else if a, ok := r.adapterMap["openai"]; ok {
		defaultAdapter = a
	}

	if defaultAdapter == nil {
		// 终极修复：如果没有任何配置，默认使用你本地存在的 qwen3.5:0.8b
		defaultAdapter = ollama.NewOllamaAdapter(ollama.Config{
			BaseURL: "http://localhost:11434", Model: "qwen3.5:0.8b",
			Temperature: 0.7, Timeout: 300 * time.Second,
		})
		r.RegisterAdapter(defaultAdapter)
	}

	// 统一注册为系统的核心能力
	llmSkill := skills.NewLLMSkill("llm", defaultAdapter, r, r.router, r.tracker, r.tracer, r.strategyStore, nil, "{{.prompt}}", r.bus)
	r.registry.Register(llmSkill)
	return llmSkill
}

package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/nikkofu/aether/internal/core/agent"
	"github.com/nikkofu/aether/internal/pkg/audit"
	"github.com/nikkofu/aether/internal/pkg/bus"
	"github.com/nikkofu/aether/internal/core/capability"
	"github.com/nikkofu/aether/internal/core/capability/skills"
	"github.com/nikkofu/aether/internal/adapters/capabilities"
	cap_alarm "github.com/nikkofu/aether/internal/adapters/capabilities/alarm"
	cap_calendar "github.com/nikkofu/aether/internal/adapters/capabilities/calendar"
	cap_code "github.com/nikkofu/aether/internal/adapters/capabilities/code"
	cap_diary "github.com/nikkofu/aether/internal/adapters/capabilities/diary"
	cap_expense "github.com/nikkofu/aether/internal/adapters/capabilities/expense"
	cap_file "github.com/nikkofu/aether/internal/adapters/capabilities/file"
	cap_gateway "github.com/nikkofu/aether/internal/adapters/capabilities/gateway"
	cap_reg "github.com/nikkofu/aether/internal/adapters/capabilities/registry"
	cap_search "github.com/nikkofu/aether/internal/adapters/capabilities/search"
	"github.com/nikkofu/aether/internal/adapters/llm"
	"github.com/nikkofu/aether/internal/adapters/llm/gemini"
	"github.com/nikkofu/aether/internal/adapters/llm/openai"
	"github.com/nikkofu/aether/internal/core/cluster"
	"github.com/nikkofu/aether/internal/pkg/config"
	"github.com/nikkofu/aether/internal/core/dag"
	"github.com/nikkofu/aether/internal/core/economy"
	"github.com/nikkofu/aether/internal/core/governance"
	"github.com/nikkofu/aether/internal/core/governance/constitution"
	"github.com/nikkofu/aether/internal/core/issue"
	"github.com/nikkofu/aether/internal/core/knowledge"
	"github.com/nikkofu/aether/internal/core/learning"
	"github.com/nikkofu/aether/internal/pkg/logging"
	"github.com/nikkofu/aether/internal/core/memory"
	"github.com/nikkofu/aether/internal/pkg/metrics"
	"github.com/nikkofu/aether/internal/pkg/observability"
	"github.com/nikkofu/aether/internal/pkg/observability/trace"
	"github.com/nikkofu/aether/internal/core/org"
	"github.com/nikkofu/aether/internal/core/policy"
	"github.com/nikkofu/aether/internal/core/reflection"
	"github.com/nikkofu/aether/internal/core/risk"
	"github.com/nikkofu/aether/internal/pkg/routing"
	"github.com/nikkofu/aether/internal/pkg/security/rbac"
	skill_sandbox "github.com/nikkofu/aether/internal/core/skills/sandbox"
	"github.com/nikkofu/aether/internal/core/strategy"
	"github.com/nikkofu/aether/internal/core/strategy/evolution"
	"github.com/nikkofu/aether/internal/core/strategy/strategic"
	sys_scheduler "github.com/nikkofu/aether/internal/core/system/scheduler"
)

// Runtime 是 Aether 系统的核心运行时。
type Runtime struct {
	cfg             *config.Config
	db              *sql.DB
	adapterMap      map[string]cli_adapters.Adapter
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
}

func (r *Runtime) GetAdapter(name string) (cli_adapters.Adapter, bool) {
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
		adapterMap: make(map[string]cli_adapters.Adapter),
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

		r.governanceBoard = governance.NewGovernanceBoard(r.ledger, r.constitution, r.rbac, r.audit, r.govLock, r.logger)
		r.sysScheduler = sys_scheduler.NewSystemScheduler(r.ledger, r.logger)
		r.riskGuard = risk.NewRiskGuard(r.ledger, 100000.0, 0.4, 0.2)

		// 初始化调度器
		r.scheduler = cluster.NewScheduler(r.logger, r.ledger, r.riskGuard)
		if cfg.App.Mode == "cluster-leader" {
			r.scheduler.Start(context.Background(), r.bus)
		}
	}

	r.RegisterAdapter(gemini.NewAdapterWithBinary(cfg.Runtime.GeminiCommand))
	if cfg.OpenAI.APIKey != "" {
		oa := openai.NewOpenAIAdapter(openai.Config{
			BaseURL: cfg.OpenAI.BaseURL, APIKey: cfg.OpenAI.APIKey, Model: cfg.OpenAI.Model,
			Temperature: cfg.OpenAI.Temperature, Timeout: cfg.OpenAI.Timeout,
		})
		oa.SetTracer(r.tracer)
		oa.SetTraceEngine(r.traceEngine)
		r.RegisterAdapter(oa)
	}

	llmSkill := skills.NewLLMSkill("llm", r.adapterMap["gemini"], r, r.router, r.tracker, r.tracer, r.strategyStore, nil, "{{.prompt}}")
	r.registry.Register(llmSkill)

	r.strategyEngine = evolution.NewDefaultStrategyEngine(llmSkill, r.knowledgeGraph, r.logger, r.evolutionGuard)
	r.reflector = reflection.NewLLMReflector(llmSkill)
	r.strategicPlanner = strategic.NewLLMStrategicPlanner(llmSkill, r.knowledgeGraph, r.strategyEngine)

	dm := agent.NewDefaultAgentManager(llmSkill, r.tracer, r.logger, r.bus, r.knowledgeGraph, cfg.Agent.MaxSpawnPerTask, cfg.Agent.MaxConcurrency, r.traceEngine)
	dm.SetLearning(r.reflector, r.reflectionStore, r.learningEngine)
	dm.SetScheduler(r.scheduler)
	dm.RegisterRole("operational", func(ctx context.Context, name string, payload map[string]any) (agent.Agent, error) {

		return org.NewOperationalWorkerAgent(name, "tactical_manager", llmSkill, r.reflector, r.ledger, r.wasmExecutor, r.traceEngine), nil
	})
	r.agentManager = dm
	r.strategicEngine = strategic.NewEngine(r.strategicPlanner, r.strategicStore, r.agentManager, r.bus, r.logger, r.traceEngine)

	r.initSystemAgents()
	r.initOrgAgents()

	if r.sysScheduler != nil { go r.sysScheduler.Start(context.Background()) }

	// 如果是集群模式，启动心跳和相关的总线订阅
	if cfg.App.Mode == "cluster-worker" {
		cluster.StartWorkerHeartbeat(context.Background(), r.bus, cfg.App.Role, cfg.App.NodeID, r.logger)

		// 订阅针对该节点的系统任务
		r.bus.SubscribeToSubject(context.Background(), cfg.App.NodeID, func(msg agent.Message) {
			if msg.Type == "system.spawn" {
				role, _ := msg.Payload["role"].(string)
				payload, _ := msg.Payload["payload"].(map[string]any)
				if role != "" {
					r.agentManager.Spawn(context.Background(), role, payload)
				}
			}
		})
	}

	return r
}

func (r *Runtime) initSystemAgents() {
	supervisor := agent.NewSupervisorAgent("supervisor", r.tracer, r.logger)
	r.agentManager.Register(supervisor)
	r.bus.Subscribe(supervisor)

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

func (r *Runtime) RegisterAdapter(a cli_adapters.Adapter) {
	if a == nil { return }
	r.adapterMap[a.Name()] = a
	names := make([]string, 0, len(r.adapterMap))
	for n := range r.adapterMap { names = append(names, n) }
	r.router.UpdateAdapters(names)
}

func (r *Runtime) StartAgents(ctx context.Context)          { r.bus.Start(ctx) }
func (r *Runtime) GetBus() bus.Bus                          { return r.bus }
func (r *Runtime) AgentManager() agent.AgentManager         { return r.agentManager }
func (r *Runtime) Logger() logging.Logger                   { return r.logger }
func (r *Runtime) Config() *config.Config                   { return r.cfg }
func (r *Runtime) Execute(ctx context.Context, adapter, prompt string) (string, error) {
	a, ok := r.adapterMap[adapter]; if !ok { return "", fmt.Errorf("adapter not found") }
	return a.Execute(ctx, prompt)
}
func (r *Runtime) NewPipelineExecutor(workers int) *dag.PipelineExecutor {
	_ = cluster.NewScheduler(r.logger, r.ledger, r.riskGuard)
	return dag.NewPipelineExecutor(r.registry, r.policy, r.memory, r.tracer, workers)
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

package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/domain/knowledge"
	"github.com/nikkofu/aether/internal/usecase/learning"
	"github.com/nikkofu/aether/pkg/logging"
	"github.com/nikkofu/aether/pkg/observability"
	"github.com/nikkofu/aether/pkg/observability/trace"
	"github.com/nikkofu/aether/internal/usecase/reflection"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// DefaultAgentManager 管理代理的生命周期和企业级限制。
type DefaultAgentManager struct {
	mu            sync.RWMutex
	agents        map[string]Agent
	roleFactories map[string]AgentRoleFactory
	llmSkill      capability.Capability
	tracer        observability.Tracer
	traceEngine   *trace.TraceEngine
	logger        logging.Logger
	bus           Bus
	graph         knowledge.Graph

	reflector      reflection.Reflector
	reflectionStore reflection.Store
	learningEngine  *learning.LearningEngine

	maxSpawnPerTask int
	maxConcurrency  int
	totalSpawns     int64
	totalFailures   int64
	taskSpawns      map[string]int

	scheduler       Scheduler
}

type Scheduler interface {
	SelectWorker(ctx context.Context, role string) string
}

func (m *DefaultAgentManager) SetScheduler(s Scheduler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduler = s
}

func NewDefaultAgentManager(llm capability.Capability, t observability.Tracer, l logging.Logger, b Bus, g knowledge.Graph, maxSpawn, maxConn int, te *trace.TraceEngine) *DefaultAgentManager {
	return &DefaultAgentManager{
		agents:          make(map[string]Agent),
		roleFactories:   make(map[string]AgentRoleFactory),
		taskSpawns:      make(map[string]int),
		llmSkill:        llm,
		tracer:          t,
		traceEngine:     te,
		logger:          l,
		bus:             b,
		graph:           g,
		maxSpawnPerTask: maxSpawn,
		maxConcurrency:  maxConn,
	}
}

func (m *DefaultAgentManager) RegisterRole(role string, factory AgentRoleFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roleFactories[role] = factory
}

func (m *DefaultAgentManager) SetLearning(re reflection.Reflector, rs reflection.Store, le *learning.LearningEngine) {
	m.reflector = re
	m.reflectionStore = rs
	m.learningEngine = le
}

func (m *DefaultAgentManager) Register(a Agent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[a.Name()] = a
	if m.bus != nil { a.SetBus(m.bus) }
	if p, ok := a.(*PlannerAgent); ok { p.SetManager(m) }
	
	if m.graph != nil {
		m.graph.AddEntity(context.Background(), knowledge.Entity{
			ID: a.Name(), Type: "agent", Name: a.Name(), Metadata: map[string]any{"role": a.Role()},
		}, "default")
	}
}

func (m *DefaultAgentManager) Spawn(ctx context.Context, role string, payload map[string]any) (Agent, error) {
	// Tracing: task assignment using OTel via traceEngine
	if m.traceEngine != nil {
		var span oteltrace.Span
		ctx, span = m.traceEngine.StartSpan(ctx, "task assignment: "+role)
		span.SetAttributes(
			attribute.String("role", role),
			attribute.String("task_id", fmt.Sprintf("%v", payload["task_id"])),
		)
		defer span.End()
	}

	m.mu.Lock()
	if len(m.agents) >= m.maxConcurrency {
		m.mu.Unlock()
		atomic.AddInt64(&m.totalFailures, 1)
		return nil, fmt.Errorf("concurrency limit reached")
	}

	taskID, _ := payload["task_id"].(string)
	orgID, _ := payload["org_id"].(string)
	if orgID == "" { orgID = "default" }

	if taskID != "" {
		if m.taskSpawns[taskID] >= m.maxSpawnPerTask {
			m.mu.Unlock()
			atomic.AddInt64(&m.totalFailures, 1)
			return nil, fmt.Errorf("task spawn limit reached")
		}
		m.taskSpawns[taskID]++
	}
	m.mu.Unlock()

	m.mu.RLock()
	scheduler := m.scheduler
	m.mu.RUnlock()

	if scheduler != nil {
		workerID := scheduler.SelectWorker(ctx, role)
		if workerID != "" && workerID != "local" {
			// 发布远程创建消息
			m.bus.Publish(ctx, Message{
				From: "manager",
				To:   workerID,
				Type: "system.spawn",
				Payload: map[string]any{
					"role":    role,
					"payload": payload,
				},
			})
			// 返回一个代表远程代理的 Mock 或在此处结束
			// 为了保持接口一致，这里可能需要一个 ProxyAgent 或者异步处理
			// 暂时假设本地创建作为回退或目前只处理本地
		}
	}

	m.mu.RLock()
	factory, ok := m.roleFactories[role]
	m.mu.RUnlock()

	var a Agent
	var err error
	name := fmt.Sprintf("%s-%s", role, uuid.New().String()[:8])

	if ok {
		a, err = factory(ctx, name, payload)
	} else {
		switch role {
		case "planner": 
			p := NewPlannerAgent(name, m.llmSkill, m.tracer)
			p.SetGraph(m.graph) // 注入知识库支持 RAG
			a = p
		case "coder": a = NewCoderAgent(name, m.llmSkill, m.tracer)
		case "reviewer": a = NewReviewerAgent(name, m.llmSkill, m.tracer)
		default: return nil, fmt.Errorf("unsupported role")
		}
	}

	if err != nil { return nil, err }
	if m.bus != nil { a.SetBus(m.bus) }
	
	m.mu.Lock()
	m.agents[name] = a
	m.mu.Unlock()

	atomic.AddInt64(&m.totalSpawns, 1)

	if m.graph != nil {
		m.graph.AddEntity(ctx, knowledge.Entity{ID: name, Type: "agent", Name: name, Metadata: map[string]any{"role": role, "task_id": taskID}}, orgID)
		if taskID != "" {
			m.graph.AddRelation(ctx, knowledge.Relation{ID: "spawn-"+name, FromID: taskID, ToID: name, Type: "spawned"}, orgID)
		}
	}

	return a, nil
}

func (m *DefaultAgentManager) Unregister(ctx context.Context, name string, input reflection.ReflectionInput) error {
	m.mu.Lock()
	a, ok := m.agents[name]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.agents, name)
	m.mu.Unlock()

	// 尝试从元数据获取 org_id
	orgID := "default"
	if m, ok := a.Metadata()["org_id"].(string); ok { orgID = m }

	if m.reflector != nil {
		go func() {
			r, err := m.reflector.Reflect(context.Background(), input)
			if err == nil {
				if m.reflectionStore != nil { m.reflectionStore.Save(context.Background(), r) }
				if m.learningEngine != nil { m.learningEngine.UpdateStrategy(r) }
				if m.graph != nil {
					m.graph.AddEntity(context.Background(), knowledge.Entity{ID: r.ID, Type: "reflection", Name: "Reflect:" + name, Metadata: map[string]any{"confidence": r.ConfidenceScore}}, orgID)
					m.graph.AddRelation(context.Background(), knowledge.Relation{ID: "rel-"+r.ID, FromID: name, ToID: r.ID, Type: "reflected"}, orgID)
				}
			}
		}()
	}

	return a.Shutdown(ctx)
}

func (m *DefaultAgentManager) List() []Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]Agent, 0, len(m.agents))
	for _, a := range m.agents { res = append(res, a) }
	return res
}

func (m *DefaultAgentManager) Get(name string) (Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[name]
	return a, ok
}

func (m *DefaultAgentManager) GetStats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := ManagerStats{ActiveAgents: len(m.agents), TotalSpawns: atomic.LoadInt64(&m.totalSpawns), TotalFailures: atomic.LoadInt64(&m.totalFailures), StatusCounts: make(map[Status]int)}
	for _, a := range m.agents { stats.StatusCounts[a.Status()]++ }
	return stats
}

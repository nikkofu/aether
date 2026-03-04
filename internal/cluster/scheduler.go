package cluster

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/nikkofu/aether/internal/agent"
	"github.com/nikkofu/aether/internal/bus"
	"github.com/nikkofu/aether/internal/economy"
	"github.com/nikkofu/aether/internal/logging"
	"github.com/nikkofu/aether/internal/risk"
)

// Scheduler 负责管理工作节点并进行风险感知调度。
type Scheduler struct {
	mu       sync.RWMutex
	workers  map[string][]string
	lastIdx  map[string]int
	lastSeen map[string]time.Time
	ledger   economy.Ledger
	guard    *risk.RiskGuard // 注入风险守卫
	logger   logging.Logger
}

func NewScheduler(l logging.Logger, ledger economy.Ledger, guard *risk.RiskGuard) *Scheduler {
	return &Scheduler{
		workers:  make(map[string][]string),
		lastIdx:  make(map[string]int),
		lastSeen: make(map[string]time.Time),
		logger:   l,
		ledger:   ledger,
		guard:    guard,
	}
}

func (s *Scheduler) Start(ctx context.Context, b bus.Bus) {
	// 订阅来自所有 worker 的心跳
	b.SubscribeToSubject(ctx, "leader", func(msg agent.Message) {
		if msg.Type == "heartbeat" {
			role, _ := msg.Payload["role"].(string)
			workerID, _ := msg.Payload["worker_id"].(string)
			if role != "" && workerID != "" {
				s.RegisterHeartbeat(role, workerID)
			}
		}
	})

	// 启动定期检查超时 worker 的协程
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.CheckTimeouts(15 * time.Second)
			}
		}
	}()

	if s.logger != nil {
		s.logger.Info(ctx, "集群调度器已启动")
	}
}

func (s *Scheduler) RegisterHeartbeat(role, workerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	_, exists := s.lastSeen[workerID]
	s.lastSeen[workerID] = time.Now()
	
	if !exists {
		s.workers[role] = append(s.workers[role], workerID)
		if s.logger != nil {
			s.logger.Info(context.Background(), "新工作节点已注册", logging.String("worker_id", workerID), logging.String("role", role))
		}
	}
}

func (s *Scheduler) CheckTimeouts(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for role, ids := range s.workers {
		var active []string
		for _, id := range ids {
			if now.Sub(s.lastSeen[id]) <= timeout { active = append(active, id) }
		}
		s.workers[role] = active
	}
}

// SelectWorker 增加风险熔断检查。
func (s *Scheduler) SelectWorker(ctx context.Context, role string) string {
	// 尝试从 context 提取 org_id，默认使用 "default"
	orgID := "default"
	// 这里可以扩展从 context 获取 orgID 的逻辑

	// 1. 风险熔断检查
	if s.guard != nil {
		if err := s.guard.CheckSystemHealth(ctx, orgID); err != nil {
			if s.logger != nil {
				s.logger.Error(ctx, "调度被拒绝: 系统处于风险熔断状态", logging.Err(err))
			}
			return ""
		}
	}

	s.mu.RLock()
	ids := s.workers[role]
	s.mu.RUnlock()

	if len(ids) == 0 { return "" }

	if s.ledger == nil {
		return s.roundRobin(role, ids)
	}

	type candidate struct {
		id  string
		rep float64
	}
	candidates := make([]candidate, 0, len(ids))
	for _, id := range ids {
		acc, err := s.ledger.GetAccount(ctx, orgID, id)
		if err == nil && acc.Reputation >= 0 {
			candidates = append(candidates, candidate{id: id, rep: acc.Reputation})
		}
	}

	if len(candidates) == 0 { return "" }
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].rep > candidates[j].rep })
	return candidates[0].id
}

func (s *Scheduler) roundRobin(role string, ids []string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.lastIdx[role]
	if idx >= len(ids) { idx = 0 }
	selected := ids[idx]
	s.lastIdx[role] = (idx + 1) % len(ids)
	return selected
}

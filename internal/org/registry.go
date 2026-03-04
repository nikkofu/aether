package org

import (
	"sync"
)

// InMemoryRegistry 实现了 OrgRegistry 接口，提供线程安全的组织架构维护。
type InMemoryRegistry struct {
	mu     sync.RWMutex
	agents map[string]OrgAgent
}

// NewInMemoryRegistry 创建一个新的内存注册表实例。
func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{
		agents: make(map[string]OrgAgent),
	}
}

// Register 注册一个组织代理到架构中。
func (r *InMemoryRegistry) Register(a OrgAgent) {
	if a == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[a.ID()] = a
}

// Get 根据 ID 获取特定代理。
func (r *InMemoryRegistry) Get(id string) (OrgAgent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[id]
	return a, ok
}

// GetByLevel 检索指定层级的所有活跃代理。
func (r *InMemoryRegistry) GetByLevel(level OrgLevel) []OrgAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []OrgAgent
	for _, a := range r.agents {
		if a.Level() == level {
			result = append(result, a)
		}
	}
	return result
}

// Hierarchy 返回完整的组织层级映射 (Supervisor ID -> Subordinate IDs)。
func (r *InMemoryRegistry) Hierarchy() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h := make(map[string][]string)
	for id, a := range r.agents {
		sup := a.Supervisor()
		if sup != "" {
			h[sup] = append(h[sup], id)
		}
	}
	return h
}

var _ OrgRegistry = (*InMemoryRegistry)(nil)

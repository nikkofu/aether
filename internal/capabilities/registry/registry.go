package registry

import (
	"fmt"
	"sync"

	"github.com/nikkofu/aether/internal/capabilities"
)

// Registry 实现了线程安全的能力注册与检索。
type Registry struct {
	mu           sync.RWMutex
	capabilities map[string]capabilities.Capability
}

func NewRegistry() *Registry {
	return &Registry{
		capabilities: make(map[string]capabilities.Capability),
	}
}

func (r *Registry) Register(cap capabilities.Capability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[cap.Name()] = cap
}

func (r *Registry) Get(name string) (capabilities.Capability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cap, ok := r.capabilities[name]
	if !ok {
		return nil, fmt.Errorf("未找到名为 '%s' 的能力", name)
	}
	return cap, nil
}

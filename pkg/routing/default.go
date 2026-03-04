package routing

import (
	"context"
	"fmt"
	"sync"

	"github.com/nikkofu/aether/pkg/observability"
)

// DefaultRouter 实现了 Router 接口，提供基于规则和可用性的简单智能路由。
type DefaultRouter struct {
	mu       sync.RWMutex
	adapters []string
	tracer   observability.Tracer
}

// NewDefaultRouter 创建并返回一个新的 DefaultRouter。
func NewDefaultRouter(availableAdapters []string, tracer observability.Tracer) *DefaultRouter {
	return &DefaultRouter{
		adapters: availableAdapters,
		tracer:   tracer,
	}
}

// Select 根据元数据从可用列表中选择一个最佳适配器。
func (r *DefaultRouter) Select(ctx context.Context, meta RequestMeta) (string, error) {
	if r.tracer != nil {
		var span observability.Span
		ctx, span = r.tracer.StartSpan(ctx, "Router.Select", map[string]any{
			"skill":          meta.Skill,
			"requires_fast":  meta.RequiresFast,
			"requires_cheap": meta.RequiresCheap,
		})
		defer span.End()
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.adapters) == 0 {
		return "", fmt.Errorf("没有可用的适配器进行路由")
	}

	available := make(map[string]bool)
	for _, a := range r.adapters {
		available[a] = true
	}

	// 规则逻辑
	var selected string
	if meta.RequiresFast || meta.RequiresCheap {
		if available["gemini"] {
			selected = "gemini"
		}
	}

	if selected == "" {
		priority := []string{"openai", "gemini"}
		for _, name := range priority {
			if available[name] {
				selected = name
				break
			}
		}
	}

	if selected == "" {
		selected = r.adapters[0]
	}

	return selected, nil
}

// UpdateAdapters 允许在运行时动态更新可用适配器列表。
func (r *DefaultRouter) UpdateAdapters(names []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters = names
}

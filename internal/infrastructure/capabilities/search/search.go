package search

import (
	"context"
	"time"

	"github.com/nikkofu/aether/internal/infrastructure/capabilities"
)

// SearchCapability 实现了受控的网页搜索。
type SearchCapability struct{}

func NewSearchCapability() *SearchCapability { return &SearchCapability{} }

func (c *SearchCapability) Name() string { return "web_search" }

func (c *SearchCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	// 1. 设置 3 秒超时
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query, _ := req.Params["query"].(string)
	if query == "" { return capabilities.CapabilityResponse{Success: false, Error: "empty query"}, nil }

	// 2. 模拟搜索请求
	// 在生产环境中，此处应调用外部 API (如 Google/Bing Search API)
	
	// 模拟返回
	results := []string{
		"SearchResult 1: " + query + " relevant data...",
		"SearchResult 2: information related to " + query,
	}

	return capabilities.CapabilityResponse{
		Success: true,
		Data: map[string]any{
			"results": results,
			"source":  "mock_engine",
		},
	}, nil
}

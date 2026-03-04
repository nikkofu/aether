package dag

import (
	"fmt"
)

// Node 代表 DAG 中的一个执行节点。
type Node struct {
	ID        string         `json:"id" yaml:"id"`
	Skill     string         `json:"skill" yaml:"skill"`
	Input     map[string]any `json:"input" yaml:"input"`
	DependsOn []string       `json:"depends_on" yaml:"depends_on"`
}

// Pipeline 定义了一组具有依赖关系的节点。
type Pipeline struct {
	Nodes []Node `json:"nodes" yaml:"nodes"`
}

// Validate 对 Pipeline 进行全面校验，包括唯一性、依赖合法性和环检测。
func (p *Pipeline) Validate() error {
	if len(p.Nodes) == 0 {
		return nil
	}

	nodeMap := make(map[string]*Node)
	adj := make(map[string][]string)

	// 1. 检查重复 ID 并建立索引
	for i := range p.Nodes {
		node := &p.Nodes[i]
		if _, exists := nodeMap[node.ID]; exists {
			return fmt.Errorf("发现重复的节点 ID: %s", node.ID)
		}
		nodeMap[node.ID] = node
		adj[node.ID] = node.DependsOn
	}

	// 2. 检查依赖项是否存在
	for _, node := range p.Nodes {
		for _, depID := range node.DependsOn {
			if _, exists := nodeMap[depID]; !exists {
				return fmt.Errorf("节点 '%s' 的依赖项 '%s' 不存在", node.ID, depID)
			}
		}
	}

	// 3. 环检测 (使用 DFS)
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for _, node := range p.Nodes {
		if !visited[node.ID] {
			if p.hasCycle(node.ID, adj, visited, recStack) {
				return fmt.Errorf("流水线中检测到循环依赖")
			}
		}
	}

	return nil
}

// hasCycle 是一个辅助函数，用于通过 DFS 检测图中是否存在环。
func (p *Pipeline) hasCycle(id string, adj map[string][]string, visited, recStack map[string]bool) bool {
	visited[id] = true
	recStack[id] = true

	for _, neighbor := range adj[id] {
		if !visited[neighbor] {
			if p.hasCycle(neighbor, adj, visited, recStack) {
				return true
			}
		} else if recStack[neighbor] {
			return true
		}
	}

	recStack[id] = false
	return false
}

// ToMermaid 将当前的 DAG 导出为 Mermaid.js 图表格式字符串。
func (p *Pipeline) ToMermaid() string {
	if len(p.Nodes) == 0 {
		return "graph TD\n    Empty[No Nodes]"
	}

	var mermaid string
	mermaid += "graph TD\n"
	
	// 渲染所有节点
	for _, node := range p.Nodes {
		// 为了视觉美观，将 Skill 加上方括号作为节点的显示文本
		mermaid += fmt.Sprintf("    %s[%s: %s]\n", node.ID, node.ID, node.Skill)
	}

	// 渲染依赖关系 (边)
	mermaid += "\n    %% Dependencies\n"
	for _, node := range p.Nodes {
		for _, dep := range node.DependsOn {
			mermaid += fmt.Sprintf("    %s --> %s\n", dep, node.ID)
		}
	}

	return mermaid
}


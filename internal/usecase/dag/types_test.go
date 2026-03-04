package dag

import (
	"strings"
	"testing"
)

func TestPipeline_ToMermaid(t *testing.T) {
	// 构造一个包含依赖的 Pipeline
	pipeline := Pipeline{
		Nodes: []Node{
			{ID: "task1", Skill: "read_file"},
			{ID: "task2", Skill: "llm", DependsOn: []string{"task1"}},
			{ID: "task3", Skill: "write_file", DependsOn: []string{"task2"}},
			{ID: "task4", Skill: "notify", DependsOn: []string{"task3"}},
		},
	}

	// 导出为 Mermaid 字符串
	mermaid := pipeline.ToMermaid()

	// 验证基础语法结构
	if !strings.HasPrefix(mermaid, "graph TD\n") {
		t.Errorf("期望以 'graph TD\\n' 开头, 实际得到:\n%s", mermaid)
	}

	// 验证节点渲染
	expectedNodes := []string{
		"    task1[task1: read_file]",
		"    task2[task2: llm]",
		"    task3[task3: write_file]",
		"    task4[task4: notify]",
	}
	for _, n := range expectedNodes {
		if !strings.Contains(mermaid, n) {
			t.Errorf("期望包含节点定义: %s, 实际未找到", n)
		}
	}

	// 验证边 (依赖) 渲染
	expectedEdges := []string{
		"    task1 --> task2",
		"    task2 --> task3",
		"    task3 --> task4",
	}
	for _, e := range expectedEdges {
		if !strings.Contains(mermaid, e) {
			t.Errorf("期望包含依赖关系: %s, 实际未找到", e)
		}
	}
}

func TestPipeline_ToMermaid_Empty(t *testing.T) {
	pipeline := Pipeline{Nodes: []Node{}}
	mermaid := pipeline.ToMermaid()

	expected := "graph TD\n    Empty[No Nodes]"
	if mermaid != expected {
		t.Errorf("空 Pipeline 渲染失败. 期望: %s, 得到: %s", expected, mermaid)
	}
}

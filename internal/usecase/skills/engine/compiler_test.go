package engine

import (
	"context"
	"os"
	"strings"
	"testing"

	domain_skills "github.com/nikkofu/aether/internal/domain/capability/skills"
	"github.com/nikkofu/aether/pkg/logging"
)

// mockSkillEngine 用于模拟 LLM 返回翻译后的 Go 代码。
type mockSkillEngine struct {
	expectedOutput string
}

func (m *mockSkillEngine) Execute(ctx context.Context, skillID string, input map[string]any) (map[string]any, error) {
	return map[string]any{
		"output": m.expectedOutput,
	}, nil
}

func (m *mockSkillEngine) Register(ctx context.Context, skill domain_skills.Skill) error { return nil }
func (m *mockSkillEngine) RegisterVersion(ctx context.Context, version domain_skills.SkillVersion) error { return nil }
func (m *mockSkillEngine) ActivateVersion(ctx context.Context, skillID, version string) error { return nil }
func (m *mockSkillEngine) ListActive(ctx context.Context) ([]domain_skills.Skill, error) { return nil, nil }
func (m *mockSkillEngine) GetVersion(ctx context.Context, skillID, version string) (*domain_skills.SkillVersion, error) { return nil, nil }
func (m *mockSkillEngine) ListVersions(ctx context.Context, skillID string) ([]domain_skills.SkillVersion, error) { return nil, nil }

func TestPolyglotCompiler_Compile_GoDirect(t *testing.T) {
	logger, _ := logging.NewLogger(logging.Config{Level: "error"})
	tempDir := t.TempDir()
	compiler := NewPolyglotCompiler(nil, "test_llm", logger, tempDir)

	sourceCode := `
package main

import (
	"encoding/json"
	"os"
)

func main() {
	var input map[string]any
	json.NewDecoder(os.Stdin).Decode(&input)
	
	output := map[string]any{"result": "success", "echo": input}
	json.NewEncoder(os.Stdout).Encode(output)
}
`

	wasmPath, err := compiler.Compile(context.Background(), sourceCode, "go", "test_skill")
	if err != nil {
		t.Fatalf("预期编译成功，但得到错误: %v", err)
	}

	if !strings.HasSuffix(wasmPath, ".wasm") {
		t.Errorf("预期生成 .wasm 文件，但得到: %s", wasmPath)
	}

	stat, err := os.Stat(wasmPath)
	if err != nil || stat.Size() == 0 {
		t.Error("预期生成的 WASM 文件存在且不为空")
	}
}

func TestPolyglotCompiler_Compile_PythonTranslated(t *testing.T) {
	logger, _ := logging.NewLogger(logging.Config{Level: "error"})
	
	// 模拟 LLM 成功将 Python 转换为对应的 Go 代码
	mockLLM := &mockSkillEngine{
		expectedOutput: `
package main
import (
	"encoding/json"
	"os"
)
func main() {
	var input map[string]any
	json.NewDecoder(os.Stdin).Decode(&input)
	output := map[string]any{"msg": "translated from python"}
	json.NewEncoder(os.Stdout).Encode(output)
}`,
	}

	tempDir := t.TempDir()
	compiler := NewPolyglotCompiler(mockLLM, "test_llm", logger, tempDir)

	pythonCode := `
import json
import sys

def main():
    input_data = json.load(sys.stdin)
    print(json.dumps({"msg": "translated from python"}))

if __name__ == "__main__":
    main()
`

	wasmPath, err := compiler.Compile(context.Background(), pythonCode, "python", "py_skill")
	if err != nil {
		t.Fatalf("预期编译成功，但得到错误: %v", err)
	}

	stat, err := os.Stat(wasmPath)
	if err != nil || stat.Size() == 0 {
		t.Error("预期生成的 WASM 文件存在且不为空")
	}
}

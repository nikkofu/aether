package code

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nikkofu/aether/internal/infrastructure/capabilities"
)

type CodeCapability struct{}

func NewCodeCapability() *CodeCapability { return &CodeCapability{} }

func (c *CodeCapability) Name() string { return "code_tools" }

func (c *CodeCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	action, _ := req.Params["action"].(string)
	source, _ := req.Params["source"].(string)

	if source == "" {
		return capabilities.CapabilityResponse{Success: false, Error: "empty source"}, nil
	}

	// 创建临时文件进行处理
	tmpDir := filepath.Join(os.TempDir(), "aether-code", req.OrgID)
	os.MkdirAll(tmpDir, 0755)
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("tool-%d.go", time.Now().UnixNano()))
	os.WriteFile(tmpFile, []byte(source), 0644)
	defer os.Remove(tmpFile)

	switch action {
	case "format":
		cmd := exec.CommandContext(ctx, "gofmt", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return capabilities.CapabilityResponse{Success: false, Error: string(out)}, nil
		}
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"formatted": string(out)}}, nil

	case "static_analysis":
		// 使用 go vet 示例
		cmd := exec.CommandContext(ctx, "go", "vet", tmpFile)
		out, err := cmd.CombinedOutput()
		success := err == nil
		return capabilities.CapabilityResponse{Success: success, Data: map[string]any{"report": string(out)}}, nil

	default:
		return capabilities.CapabilityResponse{Success: false, Error: "unsupported action"}, nil
	}
}

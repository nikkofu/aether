package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikkofu/aether/internal/capabilities"
)

// FileCapability 实现了受限的文件系统访问。
type FileCapability struct {
	baseDir string
}

func NewFileCapability(baseDir string) *FileCapability {
	return &FileCapability{baseDir: baseDir}
}

func (c *FileCapability) Name() string { return "file_system" }

func (c *FileCapability) Execute(ctx context.Context, req capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	// 1. 安全沙箱路径计算
	orgRoot := filepath.Join(c.baseDir, req.OrgID)
	os.MkdirAll(orgRoot, 0755)

	fileName, _ := req.Params["file"].(string)
	if fileName == "" { return capabilities.CapabilityResponse{Success: false, Error: "missing file param"}, nil }

	// 禁止绝对路径和目录穿越
	if strings.Contains(fileName, "..") || filepath.IsAbs(fileName) {
		return capabilities.CapabilityResponse{Success: false, Error: "path violation"}, nil
	}

	targetPath := filepath.Join(orgRoot, fileName)

	// 2. 分支处理逻辑
	switch req.Params["action"] {
	case "read":
		data, err := os.ReadFile(targetPath)
		if err != nil { return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil }
		return capabilities.CapabilityResponse{Success: true, Data: map[string]any{"content": string(data)}}, nil

	case "write":
		content, _ := req.Params["content"].(string)
		// 限制文件大小 5MB
		if len(content) > 5*1024*1024 {
			return capabilities.CapabilityResponse{Success: false, Error: "file too large (>5MB)"}, nil
		}
		err := os.WriteFile(targetPath, []byte(content), 0644)
		if err != nil { return capabilities.CapabilityResponse{Success: false, Error: err.Error()}, nil }
		return capabilities.CapabilityResponse{Success: true}, nil

	default:
		return capabilities.CapabilityResponse{Success: false, Error: "unknown action"}, nil
	}
}

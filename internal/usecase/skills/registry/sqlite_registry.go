package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	domain_skills "github.com/nikkofu/aether/internal/domain/capability/skills"
	"github.com/nikkofu/aether/internal/usecase/skills/sandbox"
	_ "modernc.org/sqlite"
)

// SQLiteSkillEngine 实现了 domain_skills.SkillEngine 接口，支持版本 tree 管理。
type SQLiteSkillEngine struct {
	db       *sql.DB
	executor *sandbox.WASMExecutor
	cacheDir string
}

func NewSQLiteSkillEngine(db *sql.DB, executor *sandbox.WASMExecutor, cacheDir string) (*SQLiteSkillEngine, error) {
	if db == nil { return nil, fmt.Errorf("db required") }
	if cacheDir == "" { cacheDir = "./data/wasm_cache" }
	if err := os.MkdirAll(cacheDir, 0755); err != nil { return nil, err }

	e := &SQLiteSkillEngine{db: db, executor: executor, cacheDir: cacheDir}
	if err := e.init(context.Background()); err != nil { return nil, err }
	return e, nil
}


func (e *SQLiteSkillEngine) init(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS skills (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			created_by TEXT,
			active BOOLEAN DEFAULT 1,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS skill_versions (
			skill_id TEXT NOT NULL,
			version TEXT NOT NULL,
			parent TEXT,
			code_path TEXT NOT NULL,
			entry_point TEXT,
			score REAL DEFAULT 0.0,
			active BOOLEAN DEFAULT 0,
			created_at DATETIME NOT NULL,
			PRIMARY KEY (skill_id, version),
			FOREIGN KEY (skill_id) REFERENCES skills(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_skill_versions_active ON skill_versions(skill_id, active);`,
	}
	for _, q := range queries {
		if _, err := e.db.ExecContext(ctx, q); err != nil { return err }
	}
	return nil
}

func (e *SQLiteSkillEngine) Register(ctx context.Context, s domain_skills.Skill) error {
	if s.CreatedAt.IsZero() { s.CreatedAt = time.Now() }
	query := `INSERT INTO skills (id, name, description, created_by, active, created_at)
	VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, description=excluded.description, active=excluded.active`
	_, err := e.db.ExecContext(ctx, query, s.ID, s.Name, s.Description, s.CreatedBy, s.Active, s.CreatedAt)
	return err
}

func (e *SQLiteSkillEngine) RegisterVersion(ctx context.Context, v domain_skills.SkillVersion) error {
	if v.CreatedAt.IsZero() { v.CreatedAt = time.Now() }
	query := `INSERT INTO skill_versions (skill_id, version, parent, code_path, entry_point, score, active, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := e.db.ExecContext(ctx, query, v.SkillID, v.Version, v.Parent, v.CodePath, v.EntryPoint, v.Score, v.Active, v.CreatedAt)
	return err
}

func (e *SQLiteSkillEngine) ActivateVersion(ctx context.Context, skillID, version string) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()

	// 1. 禁用该技能的所有其他版本
	_, err = tx.ExecContext(ctx, "UPDATE skill_versions SET active = 0 WHERE skill_id = ?", skillID)
	if err != nil { return err }

	// 2. 激活目标版本
	_, err = tx.ExecContext(ctx, "UPDATE skill_versions SET active = 1 WHERE skill_id = ? AND version = ?", skillID, version)
	if err != nil { return err }

	return tx.Commit()
}

func (e *SQLiteSkillEngine) ListActive(ctx context.Context) ([]domain_skills.Skill, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT id, name, description, created_by, active, created_at FROM skills WHERE active = 1")
	if err != nil { return nil, err }
	defer rows.Close()

	var results []domain_skills.Skill
	for rows.Next() {
		var s domain_skills.Skill
		rows.Scan(&s.ID, &s.Name, &s.Description, &s.CreatedBy, &s.Active, &s.CreatedAt)
		results = append(results, s)
	}
	return results, nil
}

func (e *SQLiteSkillEngine) GetVersion(ctx context.Context, skillID, version string) (*domain_skills.SkillVersion, error) {
	row := e.db.QueryRowContext(ctx, "SELECT skill_id, version, parent, code_path, entry_point, score, active, created_at FROM skill_versions WHERE skill_id = ? AND version = ?", skillID, version)
	var v domain_skills.SkillVersion
	err := row.Scan(&v.SkillID, &v.Version, &v.Parent, &v.CodePath, &v.EntryPoint, &v.Score, &v.Active, &v.CreatedAt)
	if err != nil { return nil, err }
	return &v, nil
}

func (e *SQLiteSkillEngine) ListVersions(ctx context.Context, skillID string) ([]domain_skills.SkillVersion, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT skill_id, version, parent, code_path, entry_point, score, active, created_at FROM skill_versions WHERE skill_id = ? ORDER BY created_at DESC", skillID)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []domain_skills.SkillVersion
	for rows.Next() {
		var v domain_skills.SkillVersion
		rows.Scan(&v.SkillID, &v.Version, &v.Parent, &v.CodePath, &v.EntryPoint, &v.Score, &v.Active, &v.CreatedAt)
		results = append(results, v)
	}
	return results, nil
}

func (e *SQLiteSkillEngine) Execute(ctx context.Context, skillID string, input map[string]any) (map[string]any, error) {
	if e.executor == nil {
		return nil, fmt.Errorf("WASM executor not initialized")
	}

	// 1. 获取当前活跃版本
	row := e.db.QueryRowContext(ctx, "SELECT code_path, entry_point FROM skill_versions WHERE skill_id = ? AND active = 1", skillID)
	var codePath, entryPoint string
	if err := row.Scan(&codePath, &entryPoint); err != nil {
		return nil, fmt.Errorf("active version not found for skill: %s", skillID)
	}

	// 2. 处理远程路径 (动态加载)
	localPath := codePath
	if strings.HasPrefix(codePath, "http://") || strings.HasPrefix(codePath, "https://") {
		var err error
		localPath, err = e.ensureLocalWASM(ctx, skillID, codePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load remote WASM: %w", err)
		}
	}

	// 3. 准备执行上下文 (从 input 提取 org_id, user_id)
	orgID, _ := input["org_id"].(string)
	userID, _ := input["user_id"].(string)
	if orgID == "" { orgID = "default" }
	if userID == "" { userID = "system" }

	inputData, _ := json.Marshal(input)

	// 4. 调用沙箱执行
	outputData, err := e.executor.Execute(ctx, orgID, userID, skillID, localPath, inputData)
	if err != nil {
		return nil, err
	}

	var output map[string]any
	if err := json.Unmarshal(outputData, &output); err != nil {
		// 如果输出不是 JSON，则包装为 output 字段
		return map[string]any{"output": string(outputData), "raw": true}, nil
	}

	return output, nil
}

func (e *SQLiteSkillEngine) ensureLocalWASM(ctx context.Context, skillID, url string) (string, error) {
	// 使用 URL 的哈希作为缓存文件名
	filename := fmt.Sprintf("%s_%x.wasm", skillID, url)
	localPath := filepath.Join(e.cacheDir, filename)

	// 检查是否已缓存
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	// 下载文件
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil { return "", err }

	resp, err := http.DefaultClient.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	// 写入临时文件后重命名，确保原子性
	tmpPath := localPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil { return "", err }
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	f.Close()

	if err := os.Rename(tmpPath, localPath); err != nil {
		return "", err
	}

	return localPath, nil
}

var _ domain_skills.SkillEngine = (*SQLiteSkillEngine)(nil)

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SkillRegistry 技能注册表，支持动态加载和热重载
type SkillRegistry struct {
	mu       sync.RWMutex
	skills   map[string]*Skill
	fs       *WorkspaceFS
	parser   *MarkdownSkillParser
	handlers map[string]SkillHandler // 内置handler映射
}

// NewSkillRegistry 创建注册表
func NewSkillRegistry(fs *WorkspaceFS) *SkillRegistry {
	return &SkillRegistry{
		skills:   make(map[string]*Skill),
		fs:       fs,
		parser:   &MarkdownSkillParser{},
		handlers: make(map[string]SkillHandler),
	}
}

// RegisterBuiltinHandlers 注册内置handler
func (r *SkillRegistry) RegisterBuiltinHandlers() {
	// 注册文件系统handler
	r.handlers["native:filesystem_read_file"] = r.createReadFileHandler()
	r.handlers["native:filesystem_write_file"] = r.createWriteFileHandler()
	r.handlers["native:filesystem_workspace_read_file"] = r.createReadFileHandler()
	r.handlers["native:filesystem_workspace_write_file"] = r.createWriteFileHandler()
	r.handlers["native:memory_remember"] = r.createMemoryWriteHandler()
	r.handlers["native:memory_memory_write_important"] = r.createMemoryWriteHandler()
}

// LoadFromDir 从目录加载所有.md技能定义
func (r *SkillRegistry) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read skills dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// 递归加载子目录
			subDir := filepath.Join(dir, entry.Name())
			if err := r.LoadFromDir(subDir); err != nil {
				continue // 子目录错误不影响主流程
			}
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		skill, err := r.parser.Parse(path)
		if err != nil {
			fmt.Printf("warn: parse skill %s failed: %v\n", path, err)
			continue
		}

		// 绑定handler
		handlerKey := extractHandlerKeyFromPath(path)
		// 也尝试用技能名直接匹配
		handlerKeyAlt := "native:" + skill.Name

		var handler SkillHandler
		var ok bool

		if handler, ok = r.handlers[handlerKey]; !ok {
			handler, ok = r.handlers[handlerKeyAlt]
		}

		if !ok {
			fmt.Printf("warn: no handler found for skill %s (tried: %s, %s)\n", skill.Name, handlerKey, handlerKeyAlt)
			continue
		}

		skill.Handler = handler

		r.mu.Lock()
		r.skills[skill.Name] = skill
		r.mu.Unlock()
	}

	return nil
}

// Get 获取技能
func (r *SkillRegistry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	skill, ok := r.skills[name]
	return skill, ok
}

// List 列出所有技能
func (r *SkillRegistry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		list = append(list, s)
	}
	return list
}

// ToTools 转换为Tool列表（兼容现有AgentCore）
func (r *SkillRegistry) ToTools() ([]Tool, map[string]ToolExecutor) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.skills))
	executors := make(map[string]ToolExecutor)

	for name, skill := range r.skills {
		paramsJSON, _ := json.Marshal(skill.Parameters)
		tools = append(tools, Tool{
			Name:                 name,
			Description:          skill.Description,
			ParametersJSONSchema: string(paramsJSON),
		})

		// 包装SkillHandler为ToolExecutor
		handler := skill.Handler
		executors[name] = func(args json.RawMessage) (string, error) {
			return handler(args)
		}
	}

	return tools, executors
}

// 内置handler实现
func (r *SkillRegistry) createReadFileHandler() SkillHandler {
	return func(args json.RawMessage) (string, error) {
		var payload struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return "", fmt.Errorf("parse args failed: %w", err)
		}
		if payload.Path == "" {
			return "", fmt.Errorf("path is required")
		}
		return r.fs.Read(payload.Path)
	}
}

func (r *SkillRegistry) createWriteFileHandler() SkillHandler {
	return func(args json.RawMessage) (string, error) {
		var payload struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return "", fmt.Errorf("parse args failed: %w", err)
		}
		if payload.Path == "" {
			return "", fmt.Errorf("path is required")
		}
		return "写入成功", r.fs.Write(payload.Path, payload.Content)
	}
}

func (r *SkillRegistry) createMemoryWriteHandler() SkillHandler {
	return func(args json.RawMessage) (string, error) {
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return "", fmt.Errorf("parse args failed: %w", err)
		}
		if strings.TrimSpace(payload.Content) == "" {
			return "", fmt.Errorf("content is required")
		}
		return r.writeToMemory(payload.Content)
	}
}

func (r *SkillRegistry) writeToMemory(content string) (string, error) {
	now := Now()
	entry := fmt.Sprintf("\n\n## Memo at %s\n%s\n", now.Format(time.RFC3339), content)
	if err := r.fs.Append("MEMORY.md", entry); err != nil {
		return "", fmt.Errorf("write MEMORY.md failed: %w", err)
	}
	return "写入 MEMORY.md 成功", nil
}

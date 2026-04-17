package main

import (
	"encoding/json"
)

// Skill 定义一个可动态加载的技能
type Skill struct {
	Name        string
	Description string                 // 包含使用场景和示例的完整描述
	Parameters  map[string]interface{} // JSON Schema格式
	Handler     SkillHandler           // 执行函数
	Examples    []SkillExample         // Few-shot示例
}

// SkillExample Few-shot示例
type SkillExample struct {
	Scenario string // 场景描述
	Request  string // 用户输入
	ToolCall string // 期望的工具调用JSON
}

// SkillHandler 技能处理函数签名
type SkillHandler func(args json.RawMessage) (string, error)

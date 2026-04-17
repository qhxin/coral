package main

import "embed"

const embeddedSkillsRoot = "embedded_skills"

// defaultSkillsFS 内置默认技能模板，供 workspace 初始化时拷贝。
//
//go:embed embedded_skills/*/*.md
var defaultSkillsFS embed.FS


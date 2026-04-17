package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MarkdownSkillParser 从Markdown文件解析技能定义
type MarkdownSkillParser struct{}

// Parse 解析单个.md技能文件
func (p *MarkdownSkillParser) Parse(filepath string) (*Skill, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	text := string(content)

	skill := &Skill{
		Name:        extractH1(text),              // 一级标题
		Description: extractDescription(text),     // Description章节 + Examples
		Parameters:  extractParameters(text),      // Parameters章节
		Examples:    extractExamples(text),        // Examples章节中的代码块
	}

	return skill, nil
}

// extractH1 提取一级标题 (# SkillName)
func extractH1(text string) string {
	re := regexp.MustCompile(`(?m)^#\s+(.+)$`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractDescription 提取Description章节内容，包含使用场景
func extractDescription(text string) string {
	start := strings.Index(text, "## Description")
	if start < 0 {
		return ""
	}
	section := text[start+len("## Description"):]
	section = strings.TrimLeft(section, "\r\n \t")

	// 取到下一个二级标题为止。
	end := strings.Index(section, "\n## ")
	desc := section
	if end >= 0 {
		desc = section[:end]
	}
	desc = strings.TrimSpace(desc)

	// 附加Examples作为few-shot提示
	examples := extractExamples(text)
	if len(examples) > 0 {
		desc += "\n\n## Usage Examples\n"
		for i, ex := range examples {
			desc += fmt.Sprintf("\nExample %d: %s\n```json\n%s\n```\n",
				i+1, ex.Scenario, ex.ToolCall)
		}
	}
	return desc
}

// extractParameters 从Parameters章节提取JSON Schema
func extractParameters(text string) map[string]interface{} {
	params := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}

	// 匹配 ### paramName (required/optional) + 类型 + 描述（使用 RE2 兼容写法，避免前瞻）
	re := regexp.MustCompile(`(?ms)###\s+(\w+)\s*\(([^)]+)\).*?\n- \*\*类型\*\*:\s*([^\n]+).*?\n- \*\*描述\*\*:\s*([^\n]+)`)
	matches := re.FindAllStringSubmatch(text, -1)

	properties := params["properties"].(map[string]interface{})
	var required []string

	for _, m := range matches {
		if len(m) >= 5 {
			name := strings.TrimSpace(m[1])
			requiredFlag := strings.TrimSpace(m[2])
			paramType := strings.TrimSpace(m[3])
			description := strings.TrimSpace(m[4])

			properties[name] = map[string]interface{}{
				"type":        paramType,
				"description": description,
			}

			if strings.Contains(requiredFlag, "required") {
				required = append(required, name)
			}
		}
	}

	params["required"] = required
	return params
}

// extractExamples 提取Examples章节中的JSON代码块
func extractExamples(text string) []SkillExample {
	var examples []SkillExample

	// 匹配形如：
	// ### 示例1
	// 场景: xxxx
	// ```json
	// {...}
	// ```
	// 的块，避免跨章节贪婪匹配。
	pattern := "(?ms)###\\s*[^\\n]*\\n\\s*场景:\\s*([^\\n]+)\\n\\s*" + "`" + "`" + "`" + "json\\s*\\n(.*?)\\n\\s*" + "`" + "`" + "`"
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(text, -1)

	for _, m := range matches {
		if len(m) >= 3 {
			examples = append(examples, SkillExample{
				Scenario: strings.TrimSpace(m[1]),
				ToolCall: strings.TrimSpace(m[2]),
			})
		}
	}

	return examples
}

// extractHandlerKey 从文件路径确定handler key
func extractHandlerKeyFromPath(path string) string {
	// 从文件名推断，如 filesystem/read_file.md -> native:workspace_read_file
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, ".md")

	// 根据目录结构推断命名空间
	dir := filepath.Dir(path)
	parts := strings.Split(dir, string(filepath.Separator))
	if len(parts) > 0 {
		namespace := parts[len(parts)-1]
		return fmt.Sprintf("native:%s_%s", namespace, name)
	}

	return "native:" + name
}

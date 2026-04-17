# memory_write_important

## Description
将重要信息写入长期记忆文件(MEMORY.md)，用于跨会话保留关键信息。

**何时使用:**
- 用户明确要求"记住"、"记录"、"保存"某信息
- 用户表达长期偏好、原则或习惯（如"我喜欢..."、"我讨厌..."）
- 用户提供需要跨会话保留的配置信息（如API密钥、账号信息）
- 用户说"下次记得..."

**何时不使用:**
- 临时性的事实或当前任务的状态
- 会在本轮对话中立即使用的信息
- 一般性知识（如编程语法）
- 用户没有明确要求的普通对话内容

## Parameters

### content (required)
- **类型**: string
- **描述**: 要保存的完整信息内容。应该：
  - 包含足够的上下文，将来检索时能理解含义
  - 注明信息来源和时间
  - 如果是偏好，说明具体场景
  - 简洁明了，避免冗余

## Examples

### 示例1: 记录编码偏好
用户说: "请记住我喜欢用空格缩进，不要用tab"

```json
{
  "tool_calls": [{
    "type": "function",
    "function": {
      "name": "memory_write_important",
      "arguments": {
        "content": "用户编码偏好: 使用空格缩进(4个空格)，不使用tab字符。适用于所有编程语言。"
      }
    }
  }]
}
```

### 示例2: 记录API密钥
用户说: "我的OpenAI API密钥是sk-abc123，记住它"

```json
{
  "tool_calls": [{
    "type": "function",
    "function": {
      "name": "memory_write_important",
      "arguments": {
        "content": "OpenAI API密钥: sk-abc123 (由用户于2024-01-15提供，用于代码生成任务)"
      }
    }
  }]
}
```

### 示例3: 记录架构原则
用户说: "记住，我们项目从不使用第三方ORM，只用原生的database/sql"

```json
{
  "tool_calls": [{
    "type": "function",
    "function": {
      "name": "memory_write_important",
      "arguments": {
        "content": "项目架构原则: 禁止使用第三方ORM库，数据库操作必须使用Go标准库database/sql。技术负责人制定。"
      }
    }
  }]
}
```

### 示例4: 记录用户习惯
用户说: "我习惯先写测试再写实现"

```json
{
  "tool_calls": [{
    "type": "function",
    "function": {
      "name": "memory_write_important",
      "arguments": {
        "content": "用户开发习惯: TDD实践者，偏好先编写测试用例再实现功能代码。"
      }
    }
  }]
}
```

## Notes
- 写入MEMORY.md后，这些信息会在每次对话开始时自动加载到系统提示词中
- 如果记忆内容涉及敏感信息(如密钥)，应提醒用户注意安全风险
- 记忆内容会带有时间戳，便于后续管理
- 避免保存过于琐碎的信息，只保留长期有价值的内容
- 如果用户要求"忘记"某条记忆，目前需要手动编辑MEMORY.md文件

# workspace_write_file

## Description
覆盖写入workspace目录下的相对路径文件内容。

**何时使用:**
- 用户要求创建新文件
- 用户要求修改现有文件内容
- 保存代码、配置或文档

**何时不使用:**
- 路径是绝对路径
- 路径包含".."试图逃逸workspace
- 用户没有明确授权修改文件

## Parameters

### path (required)
- **类型**: string
- **描述**: 要写入的相对路径，例如：NOTES.md、src/utils.go

### content (required)
- **类型**: string
- **描述**: 要写入文件的完整文本内容。如果是代码，确保格式正确。

## Examples

### 示例1: 创建笔记文件
用户说: "创建一个NOTES.md文件，内容是项目注意事项"

```json
{
  "tool_calls": [{
    "type": "function",
    "function": {
      "name": "workspace_write_file",
      "arguments": {
        "path": "NOTES.md",
        "content": "# 项目注意事项\n\n1. 使用Go 1.21+\n2. 遵循标准项目布局\n"
      }
    }
  }]
}
```

## Notes
- 会覆盖原有文件内容，请确认用户意图
- 写入后会返回"写入成功"确认

# workspace_read_file

## Description
读取workspace目录下的相对路径文件内容。

**何时使用:**
- 用户要求查看、读取、打开某个文件
- 需要了解文件内容以回答问题
- 检查现有代码或配置文件

**何时不使用:**
- 文件路径是绝对路径（禁止访问workspace外文件）
- 路径包含".."（禁止目录遍历）

## Parameters

### path (required)
- **类型**: string
- **描述**: 要读取的相对路径，例如：AGENT.md、src/main.go。必须是workspace根目录下的相对路径，禁止绝对路径。

## Examples

### 示例1: 读取配置文件
用户说: "请查看AGENT.md文件内容"

```json
{
  "tool_calls": [{
    "type": "function",
    "function": {
      "name": "workspace_read_file",
      "arguments": {
        "path": "AGENT.md"
      }
    }
  }]
}
```

### 示例2: 读取代码文件
用户说: "帮我看看main.go里写了什么"

```json
{
  "tool_calls": [{
    "type": "function",
    "function": {
      "name": "workspace_read_file",
      "arguments": {
        "path": "src/main.go"
      }
    }
  }]
}
```

## Notes
- 如果文件不存在，会返回错误信息
- 大文件可能会被截断，请注意文件大小

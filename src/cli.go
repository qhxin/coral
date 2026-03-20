package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CLIOption 描述一条命令行参数及其帮助信息。
type CLIOption struct {
	Long        string // 长参数名（不含前缀），例如 "workspace"
	Short       string // 短参数名（不含前缀），例如 "w"
	ArgName     string // 参数值占位符名，例如 "DIR"，若为空表示为布尔开关
	Description string // 参数说明
}

// cliOptions 维护 coral 当前支持的所有命令行参数。
// 未来如需新增参数，只需在此处追加一条配置即可自动出现在 --help 列表中。
var cliOptions = []CLIOption{
	{
		Long:        "workspace",
		Short:       "w",
		ArgName:     "DIR",
		Description: "指定 workspace 目录（默认为可执行文件同级 workspace 子目录）",
	},
	{
		Long:        "feishu",
		Short:       "",
		ArgName:     "",
		Description: "以飞书自建应用长连接(WebSocket)模式运行，需 FEISHU_APP_ID / FEISHU_APP_SECRET",
	},
	{
		Long:        "help",
		Short:       "h",
		ArgName:     "",
		Description: "显示帮助信息并退出",
	},
}

// isHelpRequested 判断命令行参数中是否包含 --help 或 -h。
func isHelpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// printHelp 打印 coral 程序的帮助信息，包括所有已注册的 CLI 参数。
func printHelp() {
	printHelpTo(os.Stdout, filepath.Base(os.Args[0]))
}

// printHelpTo 将帮助信息写入 w（exe 为展示用的程序名）。
func printHelpTo(w io.Writer, exe string) {
	fmt.Fprintf(w, "Coral - Minimal Go Agent (OpenAI JSON + local llama-server/Qwen)\n\n")
	fmt.Fprintf(w, "用法:\n")
	fmt.Fprintf(w, "  %s [选项]\n\n", exe)
	fmt.Fprintf(w, "选项:\n")
	for _, opt := range cliOptions {
		longForm := "--" + opt.Long
		shortForm := ""
		if opt.Short != "" {
			shortForm = "-" + opt.Short
		}

		// 构造形如 "-w, --workspace DIR" 的展示形式。
		namePart := ""
		if shortForm != "" {
			namePart = shortForm
		}
		if longForm != "" {
			if namePart != "" {
				namePart += ", "
			}
			namePart += longForm
		}
		if opt.ArgName != "" {
			namePart += " " + opt.ArgName
		}

		fmt.Fprintf(w, "  %-28s %s\n", namePart, opt.Description)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "环境变量（构建时从 .env.template 解析）：")
	if len(envVarHelps) == 0 {
		fmt.Fprintln(w, "  (无可用环境变量说明，请检查 .env.template 并重新构建)")
	} else {
		for _, h := range envVarHelps {
			desc := h.Description
			if desc == "" {
				desc = "(无说明，详见 .env.template)"
			}
			fmt.Fprintf(w, "  %-28s %s\n", h.Name, desc)
		}
	}
}


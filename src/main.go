package main

import (
	"fmt"
	"os"
)

// appRunConfig 用于测试时注入 bootstrap / 飞书启动逻辑。
type appRunConfig struct {
	bootstrap func() (*AgentCore, string, error)
	feishuRun func(*AgentCore) error
}

func runApp(args []string, cfg *appRunConfig) error {
	if cfg == nil {
		cfg = &appRunConfig{
			bootstrap: bootstrapAgent,
			feishuRun: runFeishuWS,
		}
	}
	if isHelpRequested(args) {
		printHelp()
		return nil
	}

	agent, workspaceDir, err := cfg.bootstrap()
	if err != nil {
		return err
	}

	if parseFeishuMode(args) {
		if err := cfg.feishuRun(agent); err != nil {
			return fmt.Errorf("飞书长连接模式: %w", err)
		}
		return nil
	}

	runCLIPrompt(agent, workspaceDir)
	return nil
}

func main() {
	if err := loadDotenvFromExecutableDir(); err != nil {
		fmt.Fprintf(os.Stderr, "warn: load .env from executable dir failed: %v\n", err)
	}
	if err := runApp(os.Args[1:], nil); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

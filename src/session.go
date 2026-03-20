package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
)

// ChatMessage 表示持久化在 session 文件中的一条对话消息（OpenAI 标准格式超集）。
type ChatMessage struct {
	Role     string                 `json:"role"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

const (
	// 默认会话汇总窗口（天），超过该窗口的历史会被转为摘要。
	defaultSummaryWindowDays = 7

	// CORVAL_SUMMARY_WINDOW_DAYS 允许通过环境变量覆盖默认窗口天数（仅接受正整数）。
	envSummaryWindowDays = "CORVAL_SUMMARY_WINDOW_DAYS"
)

func summaryWindowDaysFromEnv() int {
	raw := strings.TrimSpace(os.Getenv(envSummaryWindowDays))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// sessionDir 返回某个 session 的目录相对路径（基于 workspace 根）。
func sessionDir(sessionID string) string {
	// 简单替换，避免出现路径分隔符等非法字符。
	cleanID := sessionID
	if cleanID == "" {
		cleanID = "default"
	}
	cleanID = filepath.Clean(cleanID)
	cleanID = filepath.Base(cleanID)
	return filepath.Join("sessions", cleanID)
}

// weeklyFilename 根据给定时间计算周归档文件名，例如 2026-03-W03.json。
func weeklyFilename(t time.Time) string {
	year, week := t.ISOWeek()
	month := int(t.Month())
	return fmt.Sprintf("%04d-%02d-W%02d.json", year, month, week)
}

// readSessionMessages 从 workspace 相对路径读取 JSON 数组形式的 ChatMessage 列表。
func readSessionMessages(fs *WorkspaceFS, relPath string) ([]ChatMessage, error) {
	content, err := fs.Read(relPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []ChatMessage{}, nil
		}
		log.Printf("error: readSessionMessages failed for %s: %v", relPath, err)
		return nil, err
	}
	if strings.TrimSpace(content) == "" {
		return []ChatMessage{}, nil
	}
	var msgs []ChatMessage
	if err := json.Unmarshal([]byte(content), &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// writeSessionMessages 将 ChatMessage 列表写回 workspace 相对路径。
func writeSessionMessages(fs *WorkspaceFS, relPath string, msgs []ChatMessage) error {
	data, err := json.MarshalIndent(msgs, "", "  ")
	if err != nil {
		log.Printf("error: writeSessionMessages marshal failed for %s: %v", relPath, err)
		return err
	}
	if err := fs.Write(relPath, string(data)); err != nil {
		log.Printf("error: writeSessionMessages write failed for %s: %v", relPath, err)
		return err
	}
	return nil
}

// newUserMessage 构造带时间戳的 user 消息。
func newUserMessage(content string, ts time.Time, meta map[string]interface{}) ChatMessage {
	if meta == nil {
		meta = map[string]interface{}{}
	}
	if _, ok := meta["timestamp"]; !ok {
		meta["timestamp"] = ts.Format(time.RFC3339)
	}
	return ChatMessage{
		Role:     "user",
		Content:  content,
		Metadata: meta,
	}
}

// newAssistantMessage 构造带时间戳的 assistant 消息。
func newAssistantMessage(content string, ts time.Time, meta map[string]interface{}) ChatMessage {
	if meta == nil {
		meta = map[string]interface{}{}
	}
	if _, ok := meta["timestamp"]; !ok {
		meta["timestamp"] = ts.Format(time.RFC3339)
	}
	return ChatMessage{
		Role:     "assistant",
		Content:  content,
		Metadata: meta,
	}
}

// appendToSessionFiles 将一轮对话写入周文件和 active.json，并维护 7 天窗口与摘要。
func appendToSessionFiles(agent *AgentCore, sessionID string, userMsg, assistantMsg ChatMessage, now time.Time) error {
	if agent == nil || agent.FS == nil {
		return nil
	}
	fs := agent.FS
	sDir := sessionDir(sessionID)

	// 1. 周文件写入
	weekly := weeklyFilename(now)
	weeklyPath := filepath.Join(sDir, weekly)
	weeklyMsgs, err := readSessionMessages(fs, weeklyPath)
	if err != nil {
		log.Printf("error: appendToSessionFiles read weekly file %s failed: %v", weeklyPath, err)
		return err
	}
	weeklyMsgs = append(weeklyMsgs, userMsg, assistantMsg)
	if err := writeSessionMessages(fs, weeklyPath, weeklyMsgs); err != nil {
		log.Printf("error: appendToSessionFiles write weekly file %s failed: %v", weeklyPath, err)
		return err
	}

	// 2. active.json 写入与压缩
	activePath := filepath.Join(sDir, "active.json")
	activeMsgs, err := readSessionMessages(fs, activePath)
	if err != nil {
		log.Printf("error: appendToSessionFiles read active file %s failed: %v", activePath, err)
		return err
	}
	activeMsgs = append(activeMsgs, userMsg, assistantMsg)
	activeMsgs, err = compactActiveMessages(agent, activeMsgs, now)
	if err != nil {
		log.Printf("error: appendToSessionFiles compactActiveMessages failed for session %s: %v", sessionID, err)
		return err
	}
	return writeSessionMessages(fs, activePath, activeMsgs)
}

// compactActiveMessages 对 active 消息执行 7 天窗口与历史摘要压缩。
func compactActiveMessages(agent *AgentCore, msgs []ChatMessage, now time.Time) ([]ChatMessage, error) {
	windowDays := defaultSummaryWindowDays
	if envDays := summaryWindowDaysFromEnv(); envDays > 0 {
		windowDays = envDays
	}
	if agent != nil && agent.SummaryWindowDays > 0 {
		windowDays = agent.SummaryWindowDays
	}
	if windowDays <= 0 {
		return msgs, nil
	}

	cutoff := now.AddDate(0, 0, -windowDays)

	var recent []ChatMessage
	var expired []ChatMessage

	for _, m := range msgs {
		if m.Role == "system" {
			recent = append(recent, m)
			continue
		}
		tsStr, _ := m.Metadata["timestamp"].(string)
		if tsStr == "" {
			recent = append(recent, m)
			continue
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			recent = append(recent, m)
			continue
		}
		if ts.Before(cutoff) {
			expired = append(expired, m)
		} else {
			recent = append(recent, m)
		}
	}

	if len(expired) == 0 || agent == nil || agent.Client == nil {
		return recent, nil
	}

	summaryText, from, to, err := summarizeMessagesWithLLM(agent, expired, now)
	if err != nil || strings.TrimSpace(summaryText) == "" {
		if err != nil {
			log.Printf("warn: summarizeMessagesWithLLM failed, skip summarizing expired history: %v", err)
		}
		return recent, nil
	}

	meta := map[string]interface{}{
		"type": "history_summary",
		"from": from.Format(time.RFC3339),
		"to":   to.Format(time.RFC3339),
	}
	summaryMsg := ChatMessage{
		Role:     "system",
		Content:  summaryText,
		Metadata: meta,
	}

	out := []ChatMessage{summaryMsg}
	out = append(out, recent...)
	return out, nil
}

// summarizeMessagesWithLLM 使用当前模型对一段历史消息生成简短摘要。
func summarizeMessagesWithLLM(agent *AgentCore, msgs []ChatMessage, now time.Time) (string, time.Time, time.Time, error) {
	if len(msgs) == 0 || agent == nil || agent.Client == nil {
		return "", now, now, nil
	}

	var from, to time.Time
	for _, m := range msgs {
		tsStr, _ := m.Metadata["timestamp"].(string)
		if tsStr == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			continue
		}
		if from.IsZero() || ts.Before(from) {
			from = ts
		}
		if to.IsZero() || ts.After(to) {
			to = ts
		}
	}
	if from.IsZero() {
		from = now
	}
	if to.IsZero() {
		to = now
	}

	var historyText strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case "user":
			fmt.Fprintf(&historyText, "用户: %s\n", m.Content)
		case "assistant":
			fmt.Fprintf(&historyText, "助手: %s\n", m.Content)
		}
	}

	sys := openai.SystemMessage("你是一个会话总结助手，请用简短中文总结以下对话的要点，突出用户长期偏好、重要结论和需要跨轮次记住的信息。不要超过200字。")
	user := openai.UserMessage(historyText.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	llmMeta := newLLMRequestLogMetaFromAgent(agent, "session_weekly_summary", 256)
	resp, err := agent.Client.ChatOnce(ctx, []openai.ChatCompletionMessageParamUnion{sys, user}, nil, 256, "", "", llmMeta)
	if err != nil {
		log.Printf("error: summarizeMessagesWithLLM ChatOnce failed: %v", err)
		return "", from, to, err
	}
	if len(resp.Choices) == 0 {
		log.Printf("warn: summarizeMessagesWithLLM got empty choices from model")
		return "", from, to, nil
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), from, to, nil
}


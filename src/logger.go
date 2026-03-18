package main

import (
	"os"
	"path/filepath"
	"sync"
)

// dailyFileLogger 实现按天切分的文件日志写入器，所有 log.Printf 将写入 workspace/logs 目录下的当天文件。
type dailyFileLogger struct {
	baseDir     string
	currentDate string
	file        *os.File
	mu          sync.Mutex
}

// newDailyFileLogger 创建一个基于 workspace/logs 目录的按天归档日志写入器。
func newDailyFileLogger(baseDir string) *dailyFileLogger {
	return &dailyFileLogger{baseDir: baseDir}
}

// Write 实现 io.Writer 接口，每次写入时根据统一时区当天日期选择/轮转日志文件。
func (l *dailyFileLogger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	today := Now().Format("2006-01-02")
	if err := os.MkdirAll(l.baseDir, 0o755); err != nil {
		return 0, err
	}

	// 若日期变化或尚未打开文件，则轮转到新的日志文件。
	if l.file == nil || l.currentDate != today {
		if l.file != nil {
			_ = l.file.Close()
		}
		filename := filepath.Join(l.baseDir, today+".log")
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, err
		}
		l.file = f
		l.currentDate = today
	}

	return l.file.Write(p)
}


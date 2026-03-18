package main

import (
	"log"
	"time"
)

// asiaShanghaiLocation 返回 Asia/Shanghai 时区；失败时直接终止进程，避免静默使用错误时区。
func asiaShanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Fatalf("fatal: failed to load Asia/Shanghai location, please fix OS tzdata: %v", err)
	}
	return loc
}

// init 在程序启动时统一设置全局本地时区为 Asia/Shanghai (UTC+8)，
// 确保后续若有直接使用 time.Now() 等未显式指定时区的调用，也默认使用 UTC+8。
func init() {
	time.Local = asiaShanghaiLocation()
}

// Now 返回统一的当前时间，固定为 Asia/Shanghai (UTC+8)。
// 约定：业务代码禁止直接使用 time.Now()，一律使用 Now() 或显式基于 asiaShanghaiLocation。
func Now() time.Time {
	return time.Now().In(asiaShanghaiLocation())
}

// NowUnix 返回统一时区下的 Unix 秒时间戳，方便日志或持久化。
func NowUnix() int64 {
	return Now().Unix()
}

// NowRFC3339 返回统一时区下的 RFC3339 字符串表示。
func NowRFC3339() string {
	return Now().Format(time.RFC3339)
}


package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var corvalLoc *time.Location

// tryParseUTCOffset 尝试解析类似 `UTC+8` / `UTC+08:30` 这类偏移格式。
// 若非 UTC 偏移写法则 ok 为 false 且 err 为 nil；解析到 UTC 但数值非法时返回 err。
func tryParseUTCOffset(tz string) (loc *time.Location, ok bool, err error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return nil, false, nil
	}

	// 统一大小写后只针对前缀 `UTC` 与符号/数字做匹配。
	upper := strings.ToUpper(tz)
	re := regexp.MustCompile(`^UTC([+-])(\d{1,2})(?::?(\d{1,2}))?$`)
	m := re.FindStringSubmatch(upper)
	if m == nil {
		return nil, false, nil
	}

	sign := 1
	if m[1] == "-" {
		sign = -1
	}
	hours, atoiErr := strconv.Atoi(m[2])
	if atoiErr != nil {
		return nil, false, fmt.Errorf("invalid timezone offset hours %q in %q: %w", m[2], tz, atoiErr)
	}
	minutes := 0
	if m[3] != "" {
		minutes, atoiErr = strconv.Atoi(m[3])
		if atoiErr != nil {
			return nil, false, fmt.Errorf("invalid timezone offset minutes %q in %q: %w", m[3], tz, atoiErr)
		}
	}

	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		return nil, false, fmt.Errorf("timezone offset out of range in %q (hours=%d, minutes=%d)", tz, hours, minutes)
	}

	offsetSeconds := sign * (hours*3600 + minutes*60)
	return time.FixedZone(upper, offsetSeconds), true, nil
}

// corvalLocation 根据环境变量 TIMEZONE 解析并返回时区；
// 失败时直接终止进程，避免静默使用错误时区。
func corvalLocation() *time.Location {
	// 保证仅解析一次（init 中会先调用一次）。
	if corvalLoc != nil {
		return corvalLoc
	}

	tz := strings.TrimSpace(os.Getenv("TIMEZONE"))
	if tz == "" {
		tz = "UTC+8"
	}

	if loc, ok, parseErr := tryParseUTCOffset(tz); parseErr != nil {
		log.Fatalf("fatal: %v", parseErr)
	} else if ok {
		corvalLoc = loc
		return corvalLoc
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Fatalf("fatal: failed to load timezone %q; try using UTC+8 / UTC+08:30 or a valid IANA zone like Asia/Shanghai. error: %v", tz, err)
	}
	corvalLoc = loc
	return corvalLoc
}

// init 在程序启动时统一设置全局本地时区为 corvalLocation（默认 UTC+8），
// 确保后续若有直接使用 time.Now() 等未显式指定时区的调用，也默认使用该配置时区。
func init() {
	time.Local = corvalLocation()
}

// Now 返回统一的当前时间，使用 corvalLocation 指定的时区。
// 约定：业务代码禁止直接使用 time.Now()，一律使用 Now() 或显式基于 corvalLocation。
func Now() time.Time {
	return time.Now().In(corvalLocation())
}

// NowUnix 返回统一时区下的 Unix 秒时间戳，方便日志或持久化。
func NowUnix() int64 {
	return Now().Unix()
}

// NowRFC3339 返回统一时区下的 RFC3339 字符串表示。
func NowRFC3339() string {
	return Now().Format(time.RFC3339)
}

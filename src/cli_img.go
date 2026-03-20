package main

import "strings"

// parseCLIImgLine 解析 `/img "path with spaces" 后续问题` 或 `/img path 后续`。
// 若路径含空格，必须使用双引号。
func parseCLIImgLine(line string) (path string, rest string, ok bool) {
	const pfx = "/img "
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(strings.ToLower(line), "/img ") {
		return "", "", false
	}
	s := strings.TrimSpace(line[len(pfx):])
	if s == "" {
		return "", "", false
	}
	if s[0] == '"' {
		for i := 1; i < len(s); i++ {
			if s[i] == '"' {
				return strings.TrimSpace(s[1:i]), strings.TrimSpace(s[i+1:]), true
			}
		}
		return "", "", false
	}
	idx := strings.IndexByte(s, ' ')
	if idx == -1 {
		return s, "", true
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:]), true
}

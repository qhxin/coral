package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// UserImage 表示随用户轮次提交的原始图像（字节 + 可选 MIME）。
type UserImage struct {
	MIME string // 如 image/jpeg；空则按字节嗅探
	Data []byte
}

const (
	defaultVisionEmptyPrompt     = "请描述或分析这张图片。"
	defaultVisionMaxImageBytes   = 10 << 20 // 10 MiB
	defaultVisionTokensPerImage  = 1700
	envVisionEmptyText           = "CORAL_VISION_EMPTY_TEXT"
	envVisionMaxImageBytes       = "AGENT_VISION_MAX_IMAGE_BYTES"
	envVisionTokensPerImage      = "AGENT_VISION_TOKENS_PER_IMAGE"
	envSaveInboundMedia          = "CORAL_SAVE_INBOUND_MEDIA"
)

func visionEmptyPrompt() string {
	s := strings.TrimSpace(os.Getenv(envVisionEmptyText))
	if s != "" {
		return s
	}
	return defaultVisionEmptyPrompt
}

func visionMaxImageBytes() int {
	n := envIntOrDefault(envVisionMaxImageBytes, defaultVisionMaxImageBytes)
	if n <= 0 {
		return defaultVisionMaxImageBytes
	}
	return n
}

func visionTokensPerImage() int {
	n := envIntOrDefault(envVisionTokensPerImage, defaultVisionTokensPerImage)
	if n < 0 {
		return 0
	}
	return n
}

func saveInboundMediaEnabled() bool {
	return envIsTruthy(envSaveInboundMedia)
}

// validateUserImages 检查每张图大小；MIME 为空时用 HTTP Content-Type 规则嗅探。
func validateUserImages(images []UserImage) error {
	maxB := visionMaxImageBytes()
	for i := range images {
		if len(images[i].Data) == 0 {
			return fmt.Errorf("image %d: empty data", i)
		}
		if len(images[i].Data) > maxB {
			return fmt.Errorf("image %d: size %d exceeds limit %d bytes", i, len(images[i].Data), maxB)
		}
		if strings.TrimSpace(images[i].MIME) == "" {
			images[i].MIME = http.DetectContentType(images[i].Data)
		}
	}
	return nil
}

// imageDataURL 生成 OpenAI 兼容的 data URL（用于 vision）。
func imageDataURL(im UserImage) string {
	mime := strings.TrimSpace(im.MIME)
	if mime == "" {
		mime = http.DetectContentType(im.Data)
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(im.Data))
}

func extensionForImageMime(mime string) string {
	m := strings.TrimSpace(strings.ToLower(mime))
	switch {
	case strings.Contains(m, "png"):
		return ".png"
	case strings.Contains(m, "gif"):
		return ".gif"
	case strings.Contains(m, "webp"):
		return ".webp"
	default:
		return ".jpg"
	}
}

// persistenceUserTextForVisionTurn 生成写入会话 JSON 的纯文本（不含 base64）。
func persistenceUserTextForVisionTurn(userText string, imageCount int) string {
	userText = strings.TrimSpace(userText)
	suffix := ""
	if imageCount > 0 {
		suffix = fmt.Sprintf("\n（本条含 %d 张图片，图片仅随当轮模型请求提交）", imageCount)
	}
	if userText == "" && imageCount > 0 {
		return strings.TrimSpace(visionEmptyPrompt()) + suffix
	}
	return userText + suffix
}

package main

import (
	"strings"
	"testing"
)

func TestVisionEmptyPrompt_envOverride(t *testing.T) {
	t.Setenv(envVisionEmptyText, "  自定义视觉提示  ")
	if s := visionEmptyPrompt(); strings.TrimSpace(s) != "自定义视觉提示" {
		t.Fatal(s)
	}
}

func TestVisionEmptyPrompt_default(t *testing.T) {
	t.Setenv(envVisionEmptyText, "")
	if visionEmptyPrompt() != defaultVisionEmptyPrompt {
		t.Fatal(visionEmptyPrompt())
	}
}

func TestVisionMaxImageBytes_nonPositiveUsesDefault(t *testing.T) {
	t.Setenv(envVisionMaxImageBytes, "0")
	if visionMaxImageBytes() != defaultVisionMaxImageBytes {
		t.Fatal(visionMaxImageBytes())
	}
	t.Setenv(envVisionMaxImageBytes, "-3")
	if visionMaxImageBytes() != defaultVisionMaxImageBytes {
		t.Fatal(visionMaxImageBytes())
	}
}

func TestVisionTokensPerImage_negative(t *testing.T) {
	t.Setenv(envVisionTokensPerImage, "-1")
	if visionTokensPerImage() != 0 {
		t.Fatal(visionTokensPerImage())
	}
}

func TestExtensionForImageMime(t *testing.T) {
	if extensionForImageMime("image/png") != ".png" {
		t.Fatal()
	}
	if extensionForImageMime("IMAGE/GIF") != ".gif" {
		t.Fatal()
	}
	if extensionForImageMime("image/webp") != ".webp" {
		t.Fatal()
	}
	if extensionForImageMime("image/jpeg") != ".jpg" {
		t.Fatal()
	}
}

func TestPersistenceUserTextForVisionTurn(t *testing.T) {
	if !strings.Contains(persistenceUserTextForVisionTurn("", 1), defaultVisionEmptyPrompt) {
		t.Fatal(persistenceUserTextForVisionTurn("", 1))
	}
	t.Setenv(envVisionEmptyText, "ENV提示")
	got := persistenceUserTextForVisionTurn("", 1)
	if !strings.Contains(got, "ENV提示") || !strings.Contains(got, "1 张图片") {
		t.Fatal(got)
	}
	if persistenceUserTextForVisionTurn("  hi  ", 0) != "hi" {
		t.Fatal()
	}
}

func TestImageDataURL_emptyMIME(t *testing.T) {
	jpegSOI := []byte{0xff, 0xd8, 0xff, 0xe0}
	u := imageDataURL(UserImage{Data: jpegSOI})
	if !strings.HasPrefix(u, "data:image/jpeg;base64,") {
		t.Fatal(u)
	}
}

func TestSaveInboundMediaEnabled(t *testing.T) {
	t.Setenv(envSaveInboundMedia, "")
	if saveInboundMediaEnabled() {
		t.Fatal()
	}
	t.Setenv(envSaveInboundMedia, "1")
	if !saveInboundMediaEnabled() {
		t.Fatal()
	}
}

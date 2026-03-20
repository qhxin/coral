package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsHelpRequested(t *testing.T) {
	if isHelpRequested([]string{"--foo", "-h"}) != true {
		t.Fatal("expected true for -h")
	}
	if isHelpRequested([]string{"x", "--help"}) != true {
		t.Fatal("expected true for --help")
	}
	if isHelpRequested([]string{"-help"}) != false { // 不支持
		t.Fatal("unexpected true")
	}
	if isHelpRequested(nil) != false || isHelpRequested([]string{}) != false {
		t.Fatal("empty should be false")
	}
}

func TestPrintHelpTo_envVarHelpsNonEmpty(t *testing.T) {
	prev := envVarHelps
	t.Cleanup(func() { envVarHelps = prev })
	envVarHelps = []EnvVarHelp{{Name: "TEST_VAR", Description: "desc"}}
	var buf bytes.Buffer
	printHelpTo(&buf, "x")
	if !strings.Contains(buf.String(), "TEST_VAR") || !strings.Contains(buf.String(), "desc") {
		t.Fatal(buf.String())
	}
}

func TestPrintHelpTo(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf, "coral.exe")
	s := buf.String()
	if !strings.Contains(s, "coral.exe") || !strings.Contains(s, "--workspace") {
		t.Fatalf("unexpected help: %s", s)
	}
	if len(envVarHelps) == 0 {
		if !strings.Contains(s, "无可用环境变量说明") {
			t.Fatal("expected empty env var help branch")
		}
	}
}

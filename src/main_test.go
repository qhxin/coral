package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRunApp_help(t *testing.T) {
	if err := runApp([]string{"--help"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunApp_bootstrapError(t *testing.T) {
	errBoom := errors.New("boom")
	cfg := &appRunConfig{
		bootstrap: func() (*AgentCore, string, error) {
			return nil, "", errBoom
		},
		feishuRun: runFeishuWS,
	}
	if err := runApp([]string{}, cfg); err != errBoom {
		t.Fatalf("got %v", err)
	}
}

func TestRunApp_feishuError(t *testing.T) {
	cfg := &appRunConfig{
		bootstrap: func() (*AgentCore, string, error) {
			return &AgentCore{}, "/tmp", nil
		},
		feishuRun: func(*AgentCore) error {
			return errors.New("ws down")
		},
	}
	err := runApp([]string{"--feishu"}, cfg)
	if err == nil || !strings.Contains(err.Error(), "飞书") {
		t.Fatalf("%v", err)
	}
}

package main

import (
	"os"
	"testing"
)

func TestEnvOrDefault(t *testing.T) {
	k := "CORVAL_TEST_ENV_OR_DEFAULT"
	_ = os.Unsetenv(k)
	if envOrDefault(k, "d") != "d" {
		t.Fatal()
	}
	t.Setenv(k, "v")
	if envOrDefault(k, "d") != "v" {
		t.Fatal()
	}
}

func TestEnvIntOrDefault(t *testing.T) {
	k := "CORVAL_TEST_ENV_INT"
	_ = os.Unsetenv(k)
	if envIntOrDefault(k, 3) != 3 {
		t.Fatal()
	}
	t.Setenv(k, "42")
	if envIntOrDefault(k, 3) != 42 {
		t.Fatal()
	}
	t.Setenv(k, "nope")
	if envIntOrDefault(k, 7) != 7 {
		t.Fatal()
	}
}

func TestLLMConcurrencyWindowFromEnv(t *testing.T) {
	k := "LLM_CONCURRENCY_WINDOW"
	_ = os.Unsetenv(k)
	if llmConcurrencyWindowFromEnv() != 1 {
		t.Fatal()
	}
	t.Setenv(k, "0")
	if llmConcurrencyWindowFromEnv() != 1 {
		t.Fatal()
	}
	t.Setenv(k, "4")
	if llmConcurrencyWindowFromEnv() != 4 {
		t.Fatal()
	}
}

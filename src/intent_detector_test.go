package main

import "testing"

func TestIntentDetectorDetectAndScore(t *testing.T) {
	tools := []Tool{
		{Name: "workspace_read_file", Description: "read file content and open config file"},
		{Name: "memory_write_important", Description: "memory remember notes and write important items"},
	}
	d := NewIntentDetector(tools)

	if intent, score := d.Detect(""); intent.Type != "" || score != 0 {
		t.Fatalf("expected empty intent and zero score, got %+v %f", intent, score)
	}

	intent, score := d.Detect("please read config file and open it")
	if intent.Type != "workspace_read_file" {
		t.Fatalf("unexpected best match: %+v", intent)
	}
	if score <= 0 {
		t.Fatalf("expected positive score, got %f", score)
	}

	// Score is capped at 1.0 even with stacked bonuses.
	inputWords := tokenizeForRAG("memory remember note write")
	capped := d.calculateScore(inputWords, tools[1])
	if capped < 0 || capped > 1.0 {
		t.Fatalf("score out of range: %f", capped)
	}
}

func TestIntentDetectorExtractKeywords(t *testing.T) {
	d := NewIntentDetector(nil)
	keywords := d.extractKeywords("this tool can memory remember read file write content")

	expect := map[string]bool{
		"记住": true,
		"读取": true,
		"写入": true,
	}
	for _, kw := range keywords {
		delete(expect, kw)
	}
	if len(expect) != 0 {
		t.Fatalf("missing expected keywords: %+v", expect)
	}
}


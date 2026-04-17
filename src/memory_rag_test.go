package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRAGMemoryLoadRetrieveAndAddEntry(t *testing.T) {
	dir := t.TempDir()
	fs := &WorkspaceFS{Root: dir}

	content := "# Memory\n\n" +
		"## Memo at 2026-01-01T00:00:00Z\ngolang parser tips\n\n" +
		"## Memo at invalid-time\nanother golang parser note\n"
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewRAGMemory(fs)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if len(m.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.entries))
	}
	for _, e := range m.entries {
		if len(e.Embedding) == 0 {
			t.Fatalf("entry %q has empty embedding", e.ID)
		}
	}

	// Cover retrieve path where one entry has nil embedding and must be computed on demand.
	m.entries = append(m.entries, MemoryEntry{
		ID:      "manual",
		Content: "golang parser and tokenizer",
	})
	res := m.Retrieve("golang parser tokenizer", 5)
	if len(res) == 0 {
		t.Fatal("expected retrieval results, got none")
	}

	if err := m.AddEntry("golang parser advanced retrieval"); err != nil {
		t.Fatal(err)
	}
	if len(m.entries) < 3 {
		t.Fatalf("expected entries to grow, got %d", len(m.entries))
	}
}

func TestRAGMemoryHelpers(t *testing.T) {
	if got := cosineSimilarity([]float32{1, 0}, []float32{1, 0}); got <= 0.99 {
		t.Fatalf("expected near 1 similarity, got %f", got)
	}
	if got := cosineSimilarity([]float32{1}, []float32{1, 2}); got != 0 {
		t.Fatalf("expected 0 for mismatched length, got %f", got)
	}
	if got := cosineSimilarity([]float32{0, 0}, []float32{0, 0}); got != 0 {
		t.Fatalf("expected 0 for zero vectors, got %f", got)
	}

	norm := normalizeVector([]float32{3, 4})
	if len(norm) != 2 {
		t.Fatalf("unexpected normalized length: %d", len(norm))
	}
	if norm[0] <= 0 || norm[1] <= 0 {
		t.Fatalf("expected positive normalized values, got %+v", norm)
	}

	zeroNorm := normalizeVector([]float32{0, 0})
	if zeroNorm[0] != 0 || zeroNorm[1] != 0 {
		t.Fatalf("expected zero vector unchanged, got %+v", zeroNorm)
	}

	if got := parseMemoryEntries("no memo markers here"); len(got) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(got))
	}
}


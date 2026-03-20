package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyFileLogger_writeAndRotate(t *testing.T) {
	dir := t.TempDir()
	d1 := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	d2 := time.Date(2020, 1, 2, 12, 0, 0, 0, time.UTC)
	n := 0
	l := &dailyFileLogger{
		baseDir: dir,
		now: func() time.Time {
			if n == 0 {
				n++
				return d1
			}
			return d2
		},
	}
	defer l.Close()
	if _, err := l.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write([]byte("b")); err != nil {
		t.Fatal(err)
	}
	b1, err := os.ReadFile(filepath.Join(dir, "2020-01-01.log"))
	if err != nil || string(b1) != "a" {
		t.Fatalf("day1 %q err=%v", b1, err)
	}
	b2, err := os.ReadFile(filepath.Join(dir, "2020-01-02.log"))
	if err != nil || string(b2) != "b" {
		t.Fatalf("day2 %q err=%v", b2, err)
	}
}

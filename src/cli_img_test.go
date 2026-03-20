package main

import "testing"

func TestParseCLIImgLine(t *testing.T) {
	p, r, ok := parseCLIImgLine(`/img C:\a.png hello`)
	if !ok || p != `C:\a.png` || r != "hello" {
		t.Fatalf("got %q %q %v", p, r, ok)
	}
	p, r, ok = parseCLIImgLine(`  /img "D:\a b.png"  看图 `)
	if !ok || p != `D:\a b.png` || r != "看图" {
		t.Fatalf("quoted: %q %q %v", p, r, ok)
	}
	_, _, ok = parseCLIImgLine(`/image x`)
	if ok {
		t.Fatal("wrong command")
	}
	_, _, ok = parseCLIImgLine(`/img`)
	if ok {
		t.Fatal("empty")
	}
}

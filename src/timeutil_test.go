package main

import (
	"testing"
	"time"
)

func TestTryParseUTCOffset(t *testing.T) {
	loc, ok, err := tryParseUTCOffset("UTC+8")
	if err != nil || !ok || loc == nil {
		t.Fatalf("UTC+8: loc=%v ok=%v err=%v", loc, ok, err)
	}
	if _, o := time.Date(2020, 6, 1, 12, 0, 0, 0, loc).Zone(); o != 8*3600 {
		t.Fatalf("offset %d", o)
	}

	loc, ok, err = tryParseUTCOffset("  UTC+08:30  ")
	if err != nil || !ok {
		t.Fatal(err, ok)
	}

	_, ok, err = tryParseUTCOffset("Asia/Shanghai")
	if err != nil || ok {
		t.Fatalf("expected not UTC offset")
	}

	_, _, err = tryParseUTCOffset("UTC+99")
	if err == nil {
		t.Fatal("expected range error")
	}

	_, _, err = tryParseUTCOffset("UTC+8:99")
	if err == nil {
		t.Fatal("expected minutes error")
	}
}

func TestNowUnixAndRFC3339(t *testing.T) {
	if NowUnix() <= 0 {
		t.Fatal()
	}
	if NowRFC3339() == "" {
		t.Fatal()
	}
}

func TestCorvalLocation_IANA_afterReset(t *testing.T) {
	prev := corvalLoc
	t.Cleanup(func() {
		corvalLoc = prev
		time.Local = corvalLocation()
	})
	corvalLoc = nil
	t.Setenv("TIMEZONE", "UTC")
	loc := corvalLocation()
	time.Local = loc
	if Now().Location().String() != loc.String() {
		t.Fatal("Now should use corval location")
	}
}

package task_01

import (
	"reflect"
	"strings"
	"testing"
)

func TestSolve(t *testing.T) {
	dataset := []byte(`1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
2.2.2.2 - - [10/May/2026:11:02:02 +0000] "GET / HTTP/1.1" 200
2.2.2.2 - - [10/May/2026:11:02:02 +0000] "GET / HTTP/1.1" 200
3.3.3.3 - - [10/May/2026:11:02:03 +0000] "GET / HTTP/1.1" 200`)

	got := solve(dataset, 5)
	want := []string{"1.1.1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSolveThresholdBoundary(t *testing.T) {
	// Exactly 4 events in one second must NOT be flagged at threshold=5;
	// a 5th in the same second must flip it to flagged.
	fourEvents := []byte(`1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200`)
	if got := solve(fourEvents, 5); len(got) != 0 {
		t.Fatalf("got %v, want no flagged IPs below threshold", got)
	}

	fiveEvents := append(fourEvents, []byte(`
1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200`)...)
	got := solve(fiveEvents, 5)
	want := []string{"1.1.1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSolveDifferentSecondsDontAccumulate(t *testing.T) {
	// 5 total events for the IP, but spread one-per-second — never 5 within
	// the same second — so it must not be flagged.
	dataset := []byte(`1.1.1.1 - - [10/May/2026:11:02:01 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:02 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:03 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:04 +0000] "GET / HTTP/1.1" 200
1.1.1.1 - - [10/May/2026:11:02:05 +0000] "GET / HTTP/1.1" 200`)
	if got := solve(dataset, 5); len(got) != 0 {
		t.Fatalf("got %v, want no flagged IPs", got)
	}
}

func TestSolveUniqueIPsAcrossQualifyingSeconds(t *testing.T) {
	line := func(ip, second string) string {
		return ip + ` - - [10/May/2026:11:02:` + second + ` +0000] "GET / HTTP/1.1" 200` + "\n"
	}
	// Repeated qualifying seconds still produce only one occurrence of an IP.
	dataset := strings.Repeat(line("10.0.0.10", "01"), 5) +
		strings.Repeat(line("10.0.0.2", "01"), 5) +
		strings.Repeat(line("10.0.0.2", "02"), 5) +
		strings.Repeat(line("10.0.0.20", "01"), 6)
	want := []string{"10.0.0.10", "10.0.0.2", "10.0.0.20"}
	if got := solve([]byte(dataset), 5); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

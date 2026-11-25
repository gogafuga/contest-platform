package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSoakStatsPercentilesAndConcurrentSnapshots(t *testing.T) {
	values := make([]int64, 25)
	for i := range values {
		values[i] = int64(i + 1)
	}
	if percentile(values, 95) != 24 || percentile(nil, 95) != 0 {
		t.Fatal("incorrect percentile")
	}
	m := &soakMetrics{started: time.Now(), samples: make(map[string][]soakSample), minutes: make(map[int]map[string][]soakSample), counters: make(map[string]int)}
	var wg sync.WaitGroup
	for range 25 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				m.record("save", time.Now(), nil)
				m.increment("poll")
				m.snapshot()
			}
		}()
	}
	wg.Wait()
	m.record("hint", time.Now(), fmt.Errorf("fixture error"))
	total, minutes, counters, errors := m.snapshot()
	if total["save"].Count != 500 || total["save"].Failures != 0 || total["hint"].Failures != 1 || counters["poll"] != 500 || len(errors) != 1 || len(minutes) != 1 {
		t.Fatal("lost metric samples")
	}
	counters["poll"] = 0
	_, _, again, _ := m.snapshot()
	if again["poll"] != 500 {
		t.Fatal("snapshot mutated live counters")
	}
}

func TestSoakCompletionMarkersAndReportPrivacy(t *testing.T) {
	for _, language := range []string{"go", "python", "csharp"} {
		source, needle, _ := soakProgram(language)
		line, column, err := completionPosition(source, needle)
		if err != nil || !strings.HasSuffix(strings.Split(source, "\n")[line][:column], needle) {
			t.Fatalf("invalid %s completion position", language)
		}
	}
	data, err := json.Marshal(student{Number: 1, Token: "secret-bearer", ClientID: "private-client-id"})
	if err != nil || strings.Contains(string(data), "secret-bearer") || strings.Contains(string(data), "private-client-id") {
		t.Fatal("credentials in report")
	}
}

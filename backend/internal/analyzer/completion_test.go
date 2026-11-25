package analyzer

import (
	"encoding/json"
	"testing"
	"time"
)

func TestColdCompletionIsPendingButBounded(t *testing.T) {
	now := time.Now()
	for _, raw := range []string{"null", "[]", `{"items":[]}`} {
		s := session{completionDeadline: now.Add(15 * time.Second)}
		if !s.pendingCompletion(json.RawMessage(raw), now) {
			t.Fatal("cold empty completion marked final")
		}
		if s.pendingCompletion(json.RawMessage(raw), now.Add(16*time.Second)) {
			t.Fatal("legitimate empty list stayed pending forever")
		}
	}
	s := session{completionDeadline: now.Add(15 * time.Second)}
	if s.pendingCompletion(json.RawMessage(`{"items":[{"label":"WriteLine"}]}`), now) {
		t.Fatal("ready completion marked pending")
	}
	if s.pendingCompletion(json.RawMessage(`[]`), now) {
		t.Fatal("already ready session must allow empty results")
	}
	s = session{completionDeadline: now.Add(15 * time.Second), diagnostics: json.RawMessage(`[]`)}
	if s.pendingCompletion(json.RawMessage(`[]`), now) {
		t.Fatal("analyzed document's empty result must not be pending")
	}
}

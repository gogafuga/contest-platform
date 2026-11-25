//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"devsec-contest01/internal/runner"
)

func TestClassQueueRemainsBoundedWithTwoWorkers(t *testing.T) {
	f := setup(t)
	socket := filepath.Join(t.TempDir(), "run.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	started := make(chan struct{}, 32)
	var active, peak atomic.Int32
	expected := readTaskFile(t, "task_03", "expected.txt")
	fake := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_ = json.NewEncoder(w).Encode(runner.Result{Status: "ok", Phase: "run", Stdout: expected, DurationMS: 1})
	})}
	go fake.Serve(listener)
	t.Cleanup(func() { _ = fake.Close(); _ = os.Remove(socket) })
	f.cfg.RunnerSocket = socket
	f.startServer()
	tokens := make([]string, 33)
	ids := make([]string, 32)
	path := "/api/v1/tasks/cidr-check/runs"
	body := `{"requestId":"10000000-0000-4000-8000-000000000001","language":"go","sourceCode":"trusted queue fixture"}`
	for i := range tokens {
		tokens[i] = f.register(fmt.Sprintf("Queue student %d", i))
		f.selectTask("cidr-check", tokens[i], 200)
		if i < 32 {
			ids[i] = decode[codeRun](t, f.json("POST", path, tokens[i], body, 202)).ID
		} else {
			f.json("POST", path, tokens[i], body, 429)
		}
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("more than two executions started")
	default:
	}
	if peak.Load() != 2 {
		t.Fatal("worker limit not exercised")
	}
	// Retries are idempotent even when the queue is full; a second job for the
	// same owner remains forbidden.
	if again := decode[codeRun](t, f.json("POST", path, tokens[0], body, 202)); again.ID != ids[0] {
		t.Fatal("duplicate job admitted")
	}
	f.json("POST", path, tokens[0], `{"requestId":"10000000-0000-4000-8000-000000000002","language":"go","sourceCode":"second"}`, 429)
	once.Do(func() { close(release) })
	for i, id := range ids {
		deadline := time.Now().Add(5 * time.Second)
		for {
			run := decode[codeRun](t, f.json("GET", path+"/"+id, tokens[i], "", 200))
			if run.Status != "queued" && run.Status != "running" {
				if run.Status != "accepted" || run.Earned != 150 {
					t.Fatalf("job %d: %+v", i, run)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("queue did not drain")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if peak.Load() > 2 {
		t.Fatal("expanded waiting queue increased worker concurrency")
	}
}

//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"devsec-contest01/internal/runner"
)

type codeRun struct {
	ID, Status, Feedback string
	Passed               bool
	Earned               float64
}

func TestExecutionGradingAndIsolation(t *testing.T) {
	f := setup(t)
	socket := filepath.Join(t.TempDir(), "runner.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	expected := readTaskFile(t, "task_01", "expected.txt")
	requests := make(chan runner.Request, 4)
	fake := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req runner.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		requests <- req
		output := []byte("incorrect output")
		if req.Source == "correct fixture" {
			output = expected
		}
		_ = json.NewEncoder(w).Encode(runner.Result{Status: "ok", Phase: "run", Stdout: output, ExitCode: 0, DurationMS: 12})
	})}
	go fake.Serve(listener)
	t.Cleanup(func() { _ = fake.Close() })
	f.cfg.RunnerSocket = socket
	f.startServer()
	alice, bob := f.register("Execution Alice"), f.register("Execution Bob")
	f.selectTask("noisy-ips", alice, 200)
	path := "/api/v1/tasks/noisy-ips/runs"
	body := `{"requestId":"10000000-0000-4000-8000-000000000001","language":"python","sourceCode":"wrong fixture"}`
	f.json("POST", path, "", body, 401)
	f.json("POST", path, bob, body, 403)
	first := decode[codeRun](t, f.json("POST", path, alice, body, 202))
	poll := func(id string) codeRun {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			row := decode[codeRun](t, f.json("GET", path+"/"+id, alice, "", 200))
			if row.Status != "queued" && row.Status != "running" {
				return row
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("run did not finish")
		return codeRun{}
	}
	wrong := poll(first.ID)
	if wrong.Status != "wrong_answer" || wrong.Passed || wrong.Earned != 0 {
		t.Fatalf("wrong verdict: %+v", wrong)
	}
	f.json("GET", path+"/"+first.ID, bob, "", 404)
	again := decode[codeRun](t, f.json("POST", path, alice, body, 202))
	if again.ID != first.ID {
		t.Fatal("duplicate request created another run")
	}
	correctBody := `{"requestId":"10000000-0000-4000-8000-000000000002","language":"go","sourceCode":"correct fixture"}`
	second := decode[codeRun](t, f.json("POST", path, alice, correctBody, 202))
	correct := poll(second.ID)
	if correct.Status != "accepted" || !correct.Passed || correct.Earned != 125 {
		t.Fatalf("correct verdict: %+v", correct)
	}
	if score := f.me(alice).Scores["noisy-ips"]; score != 125 {
		t.Fatalf("score=%v", score)
	}
	f.json("POST", path, alice, `{"requestId":"10000000-0000-4000-8000-000000000003","language":"go","sourceCode":"new"}`, 409)
	for range 2 {
		select {
		case req := <-requests:
			if req.Task != "noisy-ips" || string(req.Input) != string(readTaskFile(t, "task_01", "dataset.txt")) {
				t.Fatal("runner did not receive task dataset")
			}
		case <-time.After(time.Second):
			t.Fatal("missing runner request")
		}
	}
	select {
	case <-requests:
		t.Fatal("duplicate runner job")
	default:
	}
	// A row from a stopped backend is reported explicitly, without running it again.
	participantID := f.me(bob).Participant.ID
	var orphan string
	err = f.pool.QueryRow(context.Background(), `INSERT INTO code_runs(participant_id,request_id,task_id,language,source_code) VALUES($1,'10000000-0000-4000-8000-000000000004','noisy-ips','go','orphan') RETURNING id::text`, participantID).Scan(&orphan)
	if err != nil {
		t.Fatal(err)
	}
	restored := decode[codeRun](t, f.json("GET", path+"/"+orphan, bob, "", 200))
	if restored.Status != "interrupted" {
		t.Fatalf("orphan status: %s", restored.Status)
	}
	_ = os.Remove(socket)
}

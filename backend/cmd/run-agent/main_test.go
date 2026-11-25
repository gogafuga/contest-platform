package main

import (
	"bytes"
	"devsec-contest01/internal/runner"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// These tests execute fixed trusted Python fixtures only; participant programs
// are never executed on the host. Resource-abuse checks belong inside runsc.
func TestCommandCapturesStreamsAndFailure(t *testing.T) {
	r := command([]string{"/usr/bin/python3", "-c", "import sys; print('ok'); print('problem', file=sys.stderr); sys.exit(7)"}, nil, time.Second, "run", t.TempDir())
	if r.Status != "runtime_error" || r.ExitCode != 7 || !bytes.Contains(r.Stdout, []byte("ok")) || !bytes.Contains(r.Stderr, []byte("problem")) {
		t.Fatalf("unexpected result: %+v", r)
	}
}
func TestCommandTimeout(t *testing.T) {
	r := command([]string{"/usr/bin/python3", "-c", "import time; time.sleep(1)"}, nil, 20*time.Millisecond, "run", t.TempDir())
	if r.Status != "time_limit" {
		t.Fatalf("status=%s", r.Status)
	}
}
func TestCommandBoundsOutput(t *testing.T) {
	r := command([]string{"/usr/bin/python3", "-c", "import sys; sys.stdout.write('x' * 1100000)"}, nil, time.Second, "run", t.TempDir())
	if r.Status != "output_limit" || len(r.Stdout)+len(r.Stderr) > runner.MaxOutput {
		t.Fatalf("status=%s, bytes=%d", r.Status, len(r.Stdout)+len(r.Stderr))
	}
}

func TestAnswerFileOverridesConsoleAndKeepsFallback(t *testing.T) {
	dir := t.TempDir()
	original := runner.Result{Status: "ok", Phase: "run", Stdout: []byte("debug output"), Stderr: []byte{}, ExitCode: 0}
	if got := collectAnswerFile(original, dir); string(got.Stdout) != "debug output" {
		t.Fatal("stream fallback lost")
	}
	if err := os.WriteFile(filepath.Join(dir, "stdout"), []byte("answer\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := collectAnswerFile(original, dir); got.Status != "ok" || string(got.Stdout) != "answer\n" {
		t.Fatalf("wrong file result: %+v", got)
	}
}
func TestAnswerFileRejectsLinksAndOversizedOutput(t *testing.T) {
	original := runner.Result{Status: "ok", Phase: "run"}
	dir := t.TempDir()
	path := filepath.Join(dir, "stdout")
	if err := os.Symlink("/dev/zero", path); err != nil {
		t.Fatal(err)
	}
	if got := collectAnswerFile(original, dir); got.Status != "runtime_error" {
		t.Fatal("symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, runner.MaxOutput+1), 0600); err != nil {
		t.Fatal(err)
	}
	if got := collectAnswerFile(original, dir); got.Status != "output_limit" {
		t.Fatal("large file accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if got := collectAnswerFile(original, dir); got.Status != "runtime_error" {
		t.Fatal("pipe accepted")
	}
}

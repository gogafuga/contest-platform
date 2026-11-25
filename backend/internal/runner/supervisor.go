package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const Label = "devsec.runner=true"

type Supervisor struct {
	Docker    string
	slots     chan struct{}
	failed    atomic.Bool
	mu        sync.Mutex
	cancel    map[string]context.CancelFunc
	closing   bool
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func NewSupervisor(docker string) *Supervisor {
	return &Supervisor{Docker: docker, slots: make(chan struct{}, 2), cancel: make(map[string]context.CancelFunc)}
}

// A runtime named runsc pointing at the real gVisor binary is mandatory. There
// is deliberately no runc, privileged or unconfined fallback.
func CheckRuntime(ctx context.Context, docker string) error {
	out, err := exec.CommandContext(ctx, docker, "info", "--format", "{{json .Runtimes}}").Output()
	if err != nil {
		return ErrUnavailable
	}
	var runtimes map[string]struct{ Path string }
	if json.Unmarshal(out, &runtimes) != nil {
		return ErrUnavailable
	}
	r, ok := runtimes["runsc"]
	if !ok || filepath.Base(r.Path) != "runsc" {
		return errors.New("gVisor runtime runsc is not configured")
	}
	version, err := exec.CommandContext(ctx, r.Path, "--version").Output()
	if err != nil || !strings.Contains(string(version), "runsc version") {
		return errors.New("cannot verify gVisor runsc binary")
	}
	return nil
}
func Cleanup(ctx context.Context, docker string) error {
	out, err := exec.CommandContext(ctx, docker, "ps", "-aq", "--filter", "label="+Label).Output()
	if err != nil {
		return err
	}
	for _, id := range strings.Fields(string(out)) {
		if err := exec.CommandContext(ctx, docker, "rm", "-f", id).Run(); err != nil {
			return err
		}
	}
	return nil
}
func containerArgs(name, image, language string) []string {
	fileLimit := 32 << 20
	if language == "csharp" {
		// .NET also uses RLIMIT_FSIZE to bound its executable-code mapper.
		// 32 MiB prevents Roslyn from compiling even a minimal console program.
		fileLimit = 128 << 20
	}
	return []string{"run", "--pull=never", "-i", "--name", name, "--label", Label, "--runtime=runsc", "--log-driver=none", "--network=none", "--read-only", "--user=65532:65532", "--cap-drop=ALL", "--security-opt=no-new-privileges=true", "--pids-limit=128", "--cpus=1", "--memory=1024m", "--memory-swap=1024m", "--ulimit=nofile=256:256", fmt.Sprintf("--ulimit=fsize=%d:%d", fileLimit, fileLimit), "--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=384m,mode=1777", "--tmpfs=/work:rw,noexec,nosuid,nodev,size=32m,uid=65532,gid=65532,mode=0700", "--tmpfs=/exec:rw,exec,nosuid,nodev,size=64m,uid=65532,gid=65532,mode=0700", image}
}

type bounded struct {
	buf      bytes.Buffer
	limit    int
	cancel   context.CancelFunc
	overflow bool
}

func (b *bounded) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
		b.cancel()
	}
	_, _ = b.buf.Write(p)
	return n, nil
}
func (s *Supervisor) Run(parent context.Context, request Request) (Result, error) {
	if request.Validate() != nil || s.failed.Load() {
		return Result{}, ErrUnavailable
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Result{}, ErrBusy
	}
	token := make([]byte, 12)
	if _, err := rand.Read(token); err != nil {
		return Result{}, err
	}
	name := "contest-run-" + hex.EncodeToString(token)
	ctx, cancel := context.WithTimeout(parent, 75*time.Second)
	defer cancel()
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return Result{}, ErrUnavailable
	}
	s.wg.Add(1)
	s.cancel[name] = cancel
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.cancel, name); s.mu.Unlock(); s.wg.Done() }()
	image := map[string]string{"go": "contest-run-go:local", "python": "contest-run-python:local", "csharp": "contest-run-csharp:local"}[request.Language]
	data, err := json.Marshal(request)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, s.Docker, containerArgs(name, image, request.Language)...)
	cmd.Stdin = bytes.NewReader(data)
	output := &bounded{limit: 2 << 20, cancel: cancel}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 200 * time.Millisecond
	started := time.Now()
	runErr := cmd.Run()
	// Inspect before removal: only an actual OOM flag is classified as OOM.
	inspectCtx, stopInspect := context.WithTimeout(context.Background(), 3*time.Second)
	state, _ := exec.CommandContext(inspectCtx, s.Docker, "inspect", "--format", "{{json .State}}", name).Output()
	stopInspect()
	var status struct{ OOMKilled bool }
	_ = json.Unmarshal(state, &status)
	cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := exec.CommandContext(cleanupCtx, s.Docker, "rm", "-f", name).Run(); err != nil {
		existing, checkErr := exec.CommandContext(cleanupCtx, s.Docker, "ps", "-aq", "--filter", "name=^/"+name+"$").Output()
		if checkErr != nil || len(strings.TrimSpace(string(existing))) > 0 {
			s.failed.Store(true)
			return Result{}, fmt.Errorf("cleanup failed for %s; runner stopped accepting jobs", name)
		}
	}
	if status.OOMKilled {
		return Result{Status: "memory_limit", Phase: "run", ExitCode: -1, DurationMS: time.Since(started).Milliseconds()}, nil
	}
	if output.overflow {
		return Result{Status: "output_limit", Phase: "run", ExitCode: -1}, nil
	}
	if ctx.Err() != nil {
		return Result{Status: "time_limit", Phase: "run", ExitCode: -1, DurationMS: time.Since(started).Milliseconds()}, nil
	}
	if runErr != nil {
		return Result{}, ErrUnavailable
	}
	// Untrusted child processes cannot inject extra JSON results around the agent.
	var result Result
	decoder := json.NewDecoder(bytes.NewReader(output.buf.Bytes()))
	if decoder.Decode(&result) != nil || result.Validate() != nil {
		return Result{}, ErrUnavailable
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Result{}, ErrUnavailable
	}
	return result, nil
}
func (s *Supervisor) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		for _, cancel := range s.cancel {
			cancel()
		}
		s.mu.Unlock()
		s.wg.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = Cleanup(ctx, s.Docker)
	})
}

package analyzer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type entry struct {
	released       sync.Once
	ready          chan struct{}
	s              *session
	err            error
	owner          int64
	users          int
	created        time.Time
	lastUsed       time.Time
	projectPending bool
	admissionWait  time.Duration
}
type Manager struct {
	ctx           context.Context
	Docker        string
	Runtime       string
	MaxSessions   int
	mu            sync.Mutex
	sessions      map[string]*entry
	closed        bool
	live          int
	cleanup       sync.WaitGroup
	cleanupFailed atomic.Bool
	changed       chan struct{}
	waiters       []*admission
}

func NewManager(ctx context.Context, docker, runtime string, max int) *Manager {
	m := &Manager{ctx: ctx, Docker: docker, Runtime: runtime, MaxSessions: max, sessions: make(map[string]*entry), changed: make(chan struct{})}
	go func() {
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				m.expire()
			}
		}
	}()
	return m
}
func key(r AssistRequest) string {
	return fmt.Sprintf("%d:%s:%s:%s", r.Owner, r.Task, r.ClientID, r.Language)
}
func (m *Manager) Assist(ctx context.Context, r AssistRequest) (AssistResponse, error) {
	if r.Validate() != nil || r.Owner <= 0 || r.Task == "" || len(r.Task) > 100 {
		return AssistResponse{}, errors.New("invalid request")
	}
	k := key(r)
	if r.Action == "close" {
		m.removeIf(k, nil)
		return AssistResponse{Revision: r.Revision, Result: raw(nil)}, nil
	}
	started := time.Now()
	e, created, err := m.acquire(ctx, r)
	queueMS := time.Since(started).Milliseconds()
	if errors.Is(err, errQueued) {
		return AssistResponse{Revision: r.Revision, Pending: true, Queued: true, QueueMS: m.queueElapsed(k).Milliseconds()}, nil
	}
	if err != nil {
		return AssistResponse{}, err
	}
	defer m.finished(e)
	if created && e.admissionWait > 0 {
		queueMS = e.admissionWait.Milliseconds()
	}
	started = time.Now()
	if created {
		s, err := m.start(r.Language)
		m.mu.Lock()
		e.s, e.err = s, err
		close(e.ready)
		m.signalLocked()
		m.mu.Unlock()
		if err != nil {
			m.removeIf(k, e)
			return AssistResponse{}, err
		}
	} else {
		select {
		case <-e.ready:
		case <-ctx.Done():
			return AssistResponse{}, ctx.Err()
		}
	}
	if e.err != nil {
		return AssistResponse{}, e.err
	}
	startupMS := time.Since(started).Milliseconds()
	if e.s.ctx.Err() != nil {
		m.removeIf(k, e)
		return AssistResponse{}, ErrBusy
	}
	started = time.Now()
	result, err := e.s.assist(ctx, r)
	result.QueueMS, result.StartupMS, result.AnalysisMS = queueMS, startupMS, time.Since(started).Milliseconds()
	if err == nil {
		m.mu.Lock()
		e.projectPending = result.Pending
		m.mu.Unlock()
	}
	if e.s.ctx.Err() != nil {
		m.removeIf(k, e)
		// A bounded-lifetime session is restarted with the short busy retry,
		// rather than reporting a missing supervisor and waiting ten seconds.
		if err != nil {
			return AssistResponse{}, ErrBusy
		}
	}
	return result, err
}
func (m *Manager) start(language string) (*session, error) {
	image := map[string]string{"go": "contest-gopls:local", "python": "contest-pylsp:local", "csharp": "contest-csharp-ls:local"}[language]
	if image == "" {
		return nil, ErrUnavailable
	}
	token := make([]byte, 12)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	name := "contest-analyzer-" + hex.EncodeToString(token)
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	fileLimit := 16 << 20
	if language == "csharp" {
		fileLimit = 128 << 20
	}
	args := []string{"run", "--pull=never", "--interactive", "--name", name, "--label", "devsec.analyzer=true", "--log-driver=none", "--network=none", "--read-only", "--user=65532:65532", "--cap-drop=ALL", "--security-opt=no-new-privileges=true", "--pids-limit=128", "--cpus=1", "--memory=1024m", "--memory-swap=1024m", "--ulimit=nofile=256:256", fmt.Sprintf("--ulimit=fsize=%d:%d", fileLimit, fileLimit), "--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=384m,mode=1777", "--tmpfs=/work:rw,noexec,nosuid,nodev,size=32m,uid=65532,gid=65532,mode=0700"}
	if m.Runtime != "" {
		args = append(args, "--runtime", m.Runtime)
	}
	args = append(args, image)
	cmd := exec.CommandContext(ctx, m.Docker, args...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, err
	}
	s := &session{cmd: cmd, name: name, docker: m.Docker, cancel: cancel, ctx: ctx, stdin: stdin, pending: make(map[int64]chan rpcMessage), changed: make(chan struct{}), uri: DocumentURI(language), language: language, gate: make(chan struct{}, 1), done: make(chan struct{})}
	s.last.Store(time.Now().UnixNano())
	if err = cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		return nil, ErrUnavailable
	}
	go func() { _ = cmd.Wait(); close(s.done); cancel() }()
	go s.read(stdout)
	initCtx, initCancel := context.WithTimeout(ctx, 40*time.Second)
	defer initCancel()
	if err = s.initialize(initCtx); err != nil {
		m.closeSession(s)
		return nil, ErrUnavailable
	}
	return s, nil
}

// Startup cleanup removes only containers carrying this service's fixed label.
func CleanupOrphans(ctx context.Context, docker string) error {
	out, err := exec.CommandContext(ctx, docker, "ps", "-aq", "--filter", "label=devsec.analyzer=true").Output()
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

func (m *Manager) release(e *entry) {
	e.released.Do(func() { m.mu.Lock(); m.live--; m.signalLocked(); m.mu.Unlock() })
}

func (m *Manager) closeSession(s *session) {
	if err := s.close(); err != nil {
		m.cleanupFailed.Store(true)
		log.Print(err, "; new analyzer sessions disabled until supervisor restart")
	}
}

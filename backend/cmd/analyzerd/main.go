// analyzerd is a separate privileged supervisor. Never run the public API as root.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"devsec-contest01/internal/analyzer"
	"devsec-contest01/internal/runner"
)

func main() {
	socket := flag.String("socket", "/run/contest-analyzers/lsp.sock", "private Unix socket")
	gid := flag.Int("gid", -1, "group of the unprivileged backend user (required)")
	docker := flag.String("docker", "/usr/bin/docker", "trusted Docker CLI")
	runtime := flag.String("runtime", "runsc", "required isolation runtime (runsc)")
	max := flag.Int("max-sessions", 2, "maximum concurrent analyzer containers")
	flag.Parse()
	if *runtime != "runsc" {
		log.Fatal("gVisor runsc is required; no weaker runtime fallback")
	}
	if *gid < 0 || *max < 1 || *max > 32 {
		log.Fatal("provide --gid and max-sessions between 1 and 32")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var err error
	dir := filepath.Dir(*socket)
	if err = os.MkdirAll(dir, 0750); err != nil {
		log.Fatal(err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		log.Fatal("socket directory must be a real directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || info.Mode().Perm()&0022 != 0 {
		log.Fatal("socket directory must be owned by supervisor and not writable by group/others")
	}
	// MkdirAll permissions are masked by the caller's umask (often 077).
	// Set group traversal explicitly after validating ownership and safety.
	if err = os.Chmod(dir, 0750); err != nil {
		log.Fatal(err)
	}
	if err = os.Chown(dir, os.Geteuid(), *gid); err != nil {
		log.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "service.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Fatal("another analyzer supervisor is running")
	}
	checkCtx, checkCancel := context.WithTimeout(ctx, 20*time.Second)
	err = runner.CheckRuntime(checkCtx, *docker)
	if err == nil {
		err = analyzer.CleanupOrphans(checkCtx, *docker)
	}
	checkCancel()
	if err != nil {
		log.Fatal("cannot access Docker or clean old analyzer containers: ", err)
	}
	if info, err = os.Lstat(*socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			log.Fatal("refusing to replace a non-socket file")
		}
		if err = os.Remove(*socket); err != nil {
			log.Fatal(err)
		}
	} else if !os.IsNotExist(err) {
		log.Fatal(err)
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	defer os.Remove(*socket)
	if err = os.Chown(*socket, os.Geteuid(), *gid); err != nil {
		log.Fatal(err)
	}
	if err = os.Chmod(*socket, 0660); err != nil {
		log.Fatal(err)
	}
	manager := analyzer.NewManager(ctx, *docker, *runtime, *max)
	defer manager.Close()
	slots := make(chan struct{}, manager.RequestSlots())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manager.Capacity())
	})
	mux.HandleFunc("POST /assist", func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "busy", 429)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var request analyzer.AssistRequest
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Validate() != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		callCtx, done := context.WithTimeout(r.Context(), 50*time.Second)
		defer done()
		response, err := manager.Assist(callCtx, request)
		switch err {
		case nil:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		case analyzer.ErrBusy:
			http.Error(w, "busy", 429)
		case analyzer.ErrStale:
			http.Error(w, "stale revision", 409)
		default:
			http.Error(w, "analyzer unavailable: image missing, startup failed or session expired", 503)
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 55 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
	go func() {
		<-ctx.Done()
		stop, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = server.Shutdown(stop)
	}()
	log.Printf("analyzers listening on %s; max sessions=%d", *socket, *max)
	if err = server.Serve(listener); err != http.ErrServerClosed {
		log.Print(err)
	}
}

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

	"devsec-contest01/internal/runner"
)

func main() {
	socket := flag.String("socket", "/run/contest-runner/run.sock", "private execution socket")
	gid := flag.Int("gid", -1, "backend user's group (required)")
	docker := flag.String("docker", "/usr/bin/docker", "trusted Docker CLI")
	flag.Parse()
	if *gid < 0 {
		log.Fatal("--gid is required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dir := filepath.Dir(*socket)
	if err := os.MkdirAll(dir, 0750); err != nil {
		log.Fatal(err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		log.Fatal("invalid socket directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || info.Mode().Perm()&0022 != 0 {
		log.Fatal("unsafe socket directory owner/permissions")
	}
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
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		log.Fatal("another runner is running")
	}
	check, stop := context.WithTimeout(ctx, 20*time.Second)
	err = runner.CheckRuntime(check, *docker)
	stop()
	if err != nil {
		log.Fatal("Execution disabled: ", err, ". Install gVisor; see runners/README.md")
	}
	cleanup, stop := context.WithTimeout(ctx, 20*time.Second)
	err = runner.Cleanup(cleanup, *docker)
	stop()
	if err != nil {
		log.Fatal(err)
	}
	if info, err = os.Lstat(*socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			log.Fatal("refusing to replace non-socket")
		}
		if os.Remove(*socket) != nil {
			log.Fatal("cannot remove stale socket")
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
	supervisor := runner.NewSupervisor(*docker)
	defer supervisor.Close()
	admission := make(chan struct{}, 2)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		select {
		case admission <- struct{}{}:
			defer func() { <-admission }()
		default:
			http.Error(w, "busy", 429)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		var request runner.Request
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Validate() != nil {
			http.Error(w, "invalid job", 400)
			return
		}
		result, err := supervisor.Run(r.Context(), request)
		if err == runner.ErrBusy {
			http.Error(w, "busy", 429)
			return
		}
		if err != nil {
			log.Print(err)
			http.Error(w, "runner unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
	go func() {
		<-ctx.Done()
		supervisor.Close()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("runner ready on %s; gVisor mandatory; parallel jobs=2", *socket)
	if err = server.Serve(listener); err != http.ErrServerClosed {
		log.Print(err)
	}
}

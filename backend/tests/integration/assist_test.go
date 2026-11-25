//go:build integration

package integration_test

import (
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"devsec-contest01/internal/analyzer"
)

func TestAnalyzerAuthorizationAndOwnership(t *testing.T) {
	f := setup(t)
	socket := filepath.Join(t.TempDir(), "assist.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan analyzer.AssistRequest, 2)
	supervisor := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request analyzer.AssistRequest
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid supervisor payload")
			w.WriteHeader(400)
			return
		}
		received <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"revision":1,"result":[]}`))
	})}
	go supervisor.Serve(listener)
	t.Cleanup(func() { _ = supervisor.Close() })
	f.cfg.AnalyzerSocket = socket
	f.startServer()
	token := f.register("Analyzer Student")
	body := `{"clientId":"` + strings.Repeat("a", 32) + `","language":"go","action":"completion","revision":1,"source":"package main","owner":999,"task":"other","position":{"line":0,"character":0}}`
	path := "/api/v1/tasks/noisy-ips/assist"
	f.json("POST", path, "", body, 401)
	f.json("POST", path, token, body, 403)
	f.json("POST", "/api/v1/tasks/noisy-ips/select", token, "", 200)
	f.json("POST", path, token, body, 200)
	request := <-received
	if request.Owner == 999 || request.Owner <= 0 || request.Task != "noisy-ips" {
		t.Fatalf("client forged owner/task: %+v", request)
	}
	bad := strings.Replace(body, `"completion"`, `"workspace/executeCommand"`, 1)
	f.json("POST", path, token, bad, 400)
	bad = strings.Replace(body, `"go"`, `"shell"`, 1)
	f.json("POST", path, token, bad, 400)
	f.cfg.AnalyzerSocket = filepath.Join(t.TempDir(), "missing.sock")
	f.startServer()
	f.json("POST", path, token, body, 503)
}

package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssistValidation(t *testing.T) {
	valid := AssistRequest{ClientID: strings.Repeat("a", 32), Language: "go", Action: "completion", Revision: 1}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []AssistRequest{valid, valid, valid, valid, valid}
	cases[0].Action = "workspace/executeCommand"
	cases[1].Language = "shell"
	cases[2].Source = strings.Repeat("x", MaxSourceBytes+1)
	cases[3].ClientID = "../../socket"
	cases[4].Revision = 0
	for _, r := range cases {
		if r.Validate() == nil {
			t.Fatalf("accepted invalid action/language/size/session/version: %s", r.Action)
		}
	}
}
func TestAssistClientStatusMapping(t *testing.T) {
	for _, status := range []int{429, 409, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		client := &Client{http: server.Client()}
		client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			r.URL.Scheme = "http"
			r.URL.Host = strings.TrimPrefix(server.URL, "http://")
			return http.DefaultTransport.RoundTrip(r)
		})
		_, err := client.Assist(context.Background(), AssistRequest{})
		expected := map[int]error{429: ErrBusy, 409: ErrStale, 503: ErrUnavailable}[status]
		if !errors.Is(err, expected) {
			t.Fatalf("status %d: %v", status, err)
		}
		server.Close()
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Opt-in real-LSP test: analyzes only a fixed fixture, never participant input.
func TestLocalGoplsIntegration(t *testing.T) {
	binary := os.Getenv("CONTEST_GOPLS")
	if binary == "" {
		t.Skip("set CONTEST_GOPLS to test a local language server")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/solution\n\ngo 1.26.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := "package main\nimport \"fmt\"\nfunc main() {\n    fmt.Prinln(\"hello\")\n}\n"
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "serve")
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOTELEMETRY=off", "CGO_ENABLED=0", "GOCACHE=" + filepath.Join(dir, "cache")}
	cmd.Stderr = io.Discard
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &session{cmd: cmd, ctx: ctx, cancel: cancel, stdin: stdin, pending: make(map[int64]chan rpcMessage), changed: make(chan struct{}), uri: "file://" + file, language: "go", gate: make(chan struct{}, 1), done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(s.done) }()
	go s.read(stdout)
	defer func() { _ = stdin.Close(); cancel(); <-s.done }()
	if err := s.initialize(ctx); err != nil {
		t.Fatal(err)
	}
	req := AssistRequest{Action: "diagnostics", Revision: 1, Source: source}
	var result AssistResponse
	var err error
	for range 20 {
		result, err = s.assist(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Pending {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(string(result.Result), "Prinln") {
		t.Fatalf("expected diagnostic, got %s", result.Result)
	}
	req.Action = "completion"
	req.Revision = 2
	req.Source = "package main\nfunc main() {\n    fmt.Pr\n}\n"
	req.Position = Position{Line: 2, Character: 10}
	result, err = s.assist(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Result), "Println") {
		t.Fatalf("expected Println completion, got %s", result.Result)
	}
	var list struct{ Items []json.RawMessage }
	_ = json.Unmarshal(result.Result, &list)
	if !strings.Contains(string(result.Result), "additionalTextEdits") {
		t.Fatal("completion must include autoimport edits")
	}
	req.Revision = 1
	if _, err = s.assist(ctx, req); !errors.Is(err, ErrStale) {
		t.Fatalf("expected stale error, got %v", err)
	}
	t.Log("diagnostics, completion, autoimport and stale-revision protection verified")
}

func fixtureSession(t *testing.T, binary string, args []string, language, dir string, env []string) *session {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	file := map[string]string{"python": "main.py", "csharp": "Program.cs"}[language]
	s := &session{cmd: cmd, ctx: ctx, cancel: cancel, stdin: stdin, pending: make(map[int64]chan rpcMessage), changed: make(chan struct{}), uri: "file://" + filepath.Join(dir, file), language: language, gate: make(chan struct{}, 1), done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(s.done) }()
	go s.read(stdout)
	t.Cleanup(func() { _ = stdin.Close(); cancel(); <-s.done })
	if err = s.initialize(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestLocalPythonIntegration(t *testing.T) {
	libs := os.Getenv("CONTEST_PYLSP_LIBS")
	if libs == "" {
		t.Skip("set CONTEST_PYLSP_LIBS for real Python test")
	}
	dir := t.TempDir()
	source := "print(missing_name)\n"
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	s := fixtureSession(t, "/usr/bin/python3", []string{"-m", "pylsp"}, "python", dir, []string{"PYTHONPATH=" + libs, "HOME=" + dir})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req := AssistRequest{Action: "diagnostics", Revision: 1, Source: source}
	var result AssistResponse
	var err error
	for range 20 {
		result, err = s.assist(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(result.Result), "missing_name") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(string(result.Result), "missing_name") {
		t.Fatalf("expected undefined name, got %s", result.Result)
	}
	req.Action = "completion"
	req.Revision = 2
	req.Source = "import os\nos.pa\n"
	req.Position = Position{Line: 1, Character: 5}
	result, err = s.assist(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Result), "path") {
		t.Fatalf("expected os.path, got %s", result.Result)
	}
	req.Action = "completion"
	req.Revision = 3
	req.Source = "Pat\n"
	req.Position = Position{Line: 0, Character: 3}
	result, err = s.assist(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Result), "Path") || !strings.Contains(string(result.Result), "additionalTextEdits") {
		t.Fatalf("Python autoimport edits missing: %.1500s", result.Result)
	}
	t.Log("Python autoimport edits verified")
	t.Log("Python diagnostics and completions verified")
}
func TestLocalCsharpIntegration(t *testing.T) {
	binary := os.Getenv("CONTEST_CSHARP_LS")
	sdk := os.Getenv("CONTEST_DOTNET")
	if binary == "" || sdk == "" {
		t.Skip("set CONTEST_CSHARP_LS and CONTEST_DOTNET")
	}
	dir := t.TempDir()
	source := "using System;\nclass Program {\n static void Main() { Console.WritLne(\"hello\"); }\n}\n"
	project := `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><EnableNETAnalyzers>false</EnableNETAnalyzers></PropertyGroup></Project>`
	for name, content := range map[string]string{"Program.cs": source, "Solution.csproj": project, "NuGet.Config": `<configuration><packageSources><clear /></packageSources></configuration>`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"DOTNET_ROOT=" + sdk, "PATH=" + sdk + ":/usr/bin:/bin", "DOTNET_CLI_HOME=" + dir, "DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "XDG_RUNTIME_DIR=" + dir}
	for _, args := range [][]string{{"new", "sln", "--name", "Solution", "--format", "sln", "--force"}, {"sln", "Solution.sln", "add", "Solution.csproj"}, {"restore", "Solution.csproj", "--configfile", "NuGet.Config"}} {
		cmd := exec.Command(filepath.Join(sdk, "dotnet"), args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("SDK setup: %v: %s", err, out)
		}
	}
	coldStarted := time.Now()
	s := fixtureSession(t, binary, []string{"--solution", filepath.Join(dir, "Solution.sln")}, "csharp", dir, env)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	req := AssistRequest{Action: "diagnostics", Revision: 1, Source: source}
	var result AssistResponse
	var err error
	for range 25 {
		result, err = s.assist(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(result.Result), "WritLne") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.Contains(string(result.Result), "WritLne") {
		t.Fatalf("expected C# diagnostic, got %s", result.Result)
	}
	t.Logf("C# initialization and first diagnostic: %d ms (trusted fixture, not sandbox benchmark)", time.Since(coldStarted).Milliseconds())
	req.Action = "completion"
	req.Revision = 2
	req.Source = "using System;\nclass Program {\n static void Main() { Console.Wri }\n}\n"
	req.Position = Position{Line: 2, Character: 32}
	result, err = s.assist(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Result), "WriteLine") {
		t.Fatalf("expected WriteLine, got %s", result.Result)
	}
	req.Action = "diagnostics"
	req.Revision = 3
	req.Source = "class Program {\n static void Main() { Console.WriteLine(1); }\n}\n"
	for range 12 {
		result, err = s.assist(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(result.Result), "Console") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	req.Action = "fixes"
	req.Position = Position{Line: 1, Character: 22}
	result, err = s.assist(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Result), "using System") {
		t.Fatalf("C# import fix missing: %s", result.Result)
	}
	t.Log("C# using System import fix verified")
	t.Log("C# diagnostics and completions verified")
}

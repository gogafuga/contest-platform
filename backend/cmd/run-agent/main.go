// Trusted container entrypoint. It is not used by the public backend.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"devsec-contest01/internal/runner"
)

func main() {
	var request runner.Request
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 16<<20))
	if decoder.Decode(&request) != nil || request.Validate() != nil {
		os.Exit(2)
	}
	result := execute(request)
	if result.Validate() != nil {
		os.Exit(3)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
func execute(request runner.Request) runner.Result {
	started := time.Now()
	result := runner.Result{Status: "system_error", Phase: "compile", ExitCode: -1}
	if os.Chdir("/work") != nil {
		return result
	}
	filename := map[string]string{"go": "main.go", "python": "main.py", "csharp": "Program.cs"}[request.Language]
	files := map[string][]byte{filename: []byte(request.Source), "input.txt": request.Input, request.Task + ".txt": request.Input}
	if request.Language == "go" {
		files["go.mod"] = []byte("module contest.local/solution\n\ngo 1.27.0\n")
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join("/work", name), data, 0600); err != nil {
			return result
		}
	}
	_ = os.Chmod("/work/input.txt", 0444)
	_ = os.Chmod("/work/"+request.Task+".txt", 0444)
	// Python performs syntax checking and execution in one isolated interpreter.
	if request.Language != "python" {
		compile := map[string][]string{
			"go":     {"/usr/local/go/bin/go", "build", "-trimpath", "-o", "/exec/solution", "main.go"},
			"csharp": {"/usr/local/bin/compile-csharp"},
		}[request.Language]
		result = command(compile, nil, 45*time.Second, "compile", "/work")
		if result.Status != "ok" {
			result.DurationMS = time.Since(started).Milliseconds()
			return result
		}
	}
	run := map[string][]string{
		"go":     {"/exec/solution"},
		"python": {"/usr/local/bin/python3", "-I", "/usr/local/lib/contest-python.py"},
		"csharp": {"/usr/bin/dotnet", "exec", "--runtimeconfig", "/opt/Solution.runtimeconfig.json", "/exec/Solution.dll"},
	}[request.Language]
	result = command(run, request.Input, 5*time.Second, "run", "/work")
	if request.Language == "python" && result.Status == "runtime_error" && result.ExitCode == 65 {
		result.Status = "compile_error"
		result.Phase = "compile"
	}
	result = collectAnswerFile(result, "/work")
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

// A file named stdout is an explicit answer. Programs using the stdout stream
// remain compatible when no such file exists. Never follow links or read pipes.
func collectAnswerFile(result runner.Result, directory string) runner.Result {
	if result.Status != "ok" {
		return result
	}
	file, err := os.OpenFile(filepath.Join(directory, "stdout"), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return result
	}
	fail := func(status, message string) runner.Result {
		result.Status = status
		result.ExitCode = -1
		result.Stdout = nil
		result.Stderr = []byte(message)
		return result
	}
	if err != nil {
		return fail("runtime_error", "Cannot read answer file stdout: use a regular file in the working directory.")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fail("runtime_error", "Answer file stdout must be a regular file.")
	}
	limit := runner.MaxOutput - len(result.Stderr)
	if info.Size() > int64(limit) {
		return fail("output_limit", "Answer file stdout exceeds the output limit.")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return fail("runtime_error", "Cannot read answer file stdout.")
	}
	if len(data) > limit {
		return fail("output_limit", "Answer file stdout exceeds the output limit.")
	}
	result.Stdout = data
	return result
}

type capture struct {
	mu       sync.Mutex
	n        int
	out      bytes.Buffer
	err      bytes.Buffer
	exceeded bool
	cancel   context.CancelFunc
}
type stream struct {
	capture *capture
	stderr  bool
}

func (w stream) Write(p []byte) (int, error) {
	c := w.capture
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(p)
	available := runner.MaxOutput - c.n
	if len(p) > available {
		p = p[:available]
		c.exceeded = true
		c.cancel()
	}
	if w.stderr {
		_, _ = c.err.Write(p)
	} else {
		_, _ = c.out.Write(p)
	}
	c.n += len(p)
	return n, nil
}
func command(argv []string, input []byte, limit time.Duration, phase, directory string) runner.Result {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	capture := &capture{cancel: cancel}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = directory
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stream{capture: capture}
	cmd.Stderr = stream{capture: capture, stderr: true}
	cmd.WaitDelay = 200 * time.Millisecond
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	result := runner.Result{Status: "ok", Phase: phase, Stdout: capture.out.Bytes(), Stderr: capture.err.Bytes(), ExitCode: code}
	switch {
	case capture.exceeded:
		result.Status = "output_limit"
	case ctx.Err() == context.DeadlineExceeded:
		if phase == "compile" {
			result.Status = "compile_timeout"
		} else {
			result.Status = "time_limit"
		}
	case err != nil:
		if phase == "compile" {
			result.Status = "compile_error"
		} else {
			result.Status = "runtime_error"
		}
	}
	return result
}

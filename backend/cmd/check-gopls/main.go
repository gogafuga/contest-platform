package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"devsec-contest01/internal/analyzer"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := fmt.Sprintf("contest-gopls-check-%d", time.Now().UnixNano())

	cmd := exec.CommandContext(ctx, "/usr/bin/docker",
		"run", "-i",
		"--name", name,
		"--network", "none",
		"--read-only",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges=true",
		"--pids-limit", "128",
		"--cpus", "1",
		"--memory", "768m",
		"--memory-swap", "768m",
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=384m,mode=1777",
		"--tmpfs", "/work:rw,noexec,nosuid,nodev,size=16m,uid=65532,gid=65532,mode=0700",
		"contest-gopls:local",
	)
	cmd.Stderr = os.Stderr

	// Удаляем контейнер даже при ошибке или таймауте клиента.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer cleanupCancel()

		if err := exec.CommandContext(
			cleanupCtx, "/usr/bin/docker", "rm", "-f", name,
		).Run(); err != nil && cmd.Process != nil {
			fmt.Fprintln(os.Stderr, "Проверьте удаление контейнера:", name)
		}
		if cmd.Process != nil {
			_ = cmd.Wait()
		}
	}()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	message := json.RawMessage(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"processId": null,
			"rootUri": "file:///work",
			"capabilities": {}
		}
	}`)
	if err := analyzer.WriteMessage(stdin, message); err != nil {
		return err
	}

	reader := bufio.NewReader(stdout)
	for {
		message, err := analyzer.ReadMessage(reader)
		if err != nil {
			return fmt.Errorf("read initialize response: %w", err)
		}

		var response struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(message, &response); err != nil {
			return err
		}
		if !bytes.Equal(bytes.TrimSpace(response.ID), []byte("1")) {
			continue
		}
		if len(response.Error) > 0 &&
			!bytes.Equal(bytes.TrimSpace(response.Error), []byte("null")) {
			return fmt.Errorf("initialize failed: %s", response.Error)
		}
		if len(response.Result) == 0 {
			return fmt.Errorf("initialize response has no result")
		}

		fmt.Println("LSP initialize: успешно")
		return nil
	}
}

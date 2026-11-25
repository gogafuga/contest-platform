package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
)

const MaxSource = 256 << 10
const MaxInput = 8 << 20
const MaxOutput = 1 << 20

var taskPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
var ErrUnavailable = errors.New("runner unavailable")
var ErrBusy = errors.New("runner busy")

type Request struct {
	Language string `json:"language"`
	Source   string `json:"source"`
	Input    []byte `json:"input"`
	Task     string `json:"task"`
}

func (r Request) Validate() error {
	if (r.Language != "go" && r.Language != "python" && r.Language != "csharp") || len(r.Source) == 0 || len(r.Source) > MaxSource || len(r.Input) > MaxInput || !taskPattern.MatchString(r.Task) {
		return errors.New("invalid execution request")
	}
	return nil
}

type Result struct {
	Status     string `json:"status"`
	Phase      string `json:"phase"`
	Stdout     []byte `json:"stdout"`
	Stderr     []byte `json:"stderr"`
	ExitCode   int    `json:"exitCode"`
	DurationMS int64  `json:"durationMs"`
}

func (r Result) Validate() error {
	if len(r.Stdout)+len(r.Stderr) > MaxOutput {
		return errors.New("oversized runner output")
	}
	switch r.Status {
	case "ok", "compile_error", "compile_timeout", "runtime_error", "time_limit", "output_limit", "memory_limit", "system_error":
	default:
		return errors.New("invalid execution status")
	}
	if r.Phase != "compile" && r.Phase != "run" {
		return errors.New("invalid execution phase")
	}
	return nil
}

type Client struct{ http *http.Client }

func NewClient(socket string) *Client {
	return &Client{http: &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}}
}
func (c *Client) Run(ctx context.Context, request Request) (Result, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://runner/run", bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 429 {
		return Result{}, ErrBusy
	}
	if response.StatusCode != 200 {
		return Result{}, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return Result{}, ErrUnavailable
	}
	var result Result
	if json.Unmarshal(body, &result) != nil || result.Validate() != nil {
		return Result{}, ErrUnavailable
	}
	return result, nil
}

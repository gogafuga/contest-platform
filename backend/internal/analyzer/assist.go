package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
)

const MaxSourceBytes = 256 << 10

var ErrBusy = errors.New("analyzer busy")
var errQueued = errors.New("analyzer queued")
var ErrStale = errors.New("stale editor revision")
var ErrUnavailable = errors.New("analyzer unavailable")
var clientIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type AssistRequest struct {
	Owner    int64           `json:"owner,omitempty"`
	Task     string          `json:"task,omitempty"`
	ClientID string          `json:"clientId"`
	Language string          `json:"language"`
	Action   string          `json:"action"`
	Source   string          `json:"source"`
	Revision int64           `json:"revision"`
	Position Position        `json:"position"`
	Item     json.RawMessage `json:"item,omitempty"`
}
type AssistResponse struct {
	Queued     bool            `json:"queued,omitempty"`
	QueueMS    int64           `json:"queueMs,omitempty"`
	StartupMS  int64           `json:"startupMs,omitempty"`
	AnalysisMS int64           `json:"analysisMs,omitempty"`
	Revision   int64           `json:"revision"`
	Result     json.RawMessage `json:"result"`
	Pending    bool            `json:"pending,omitempty"`
}

func DocumentURI(language string) string {
	switch language {
	case "go":
		return "file:///work/main.go"
	case "python":
		return "file:///work/main.py"
	case "csharp":
		return "file:///work/Program.cs"
	}
	return ""
}
func (r AssistRequest) Validate() error {
	if !clientIDPattern.MatchString(r.ClientID) || DocumentURI(r.Language) == "" || len(r.Source) > MaxSourceBytes || r.Revision < 1 || r.Position.Line < 0 || r.Position.Character < 0 || r.Position.Line > MaxSourceBytes || r.Position.Character > MaxSourceBytes {
		return errors.New("invalid analyzer request")
	}
	switch r.Action {
	case "diagnostics", "completion", "resolveCompletion", "hover", "signature", "format", "imports", "fixes", "resolveAction", "close":
	default:
		return errors.New("unsupported analyzer action")
	}
	if (r.Action == "resolveCompletion" || r.Action == "resolveAction") && (len(r.Item) == 0 || len(r.Item) > 128<<10 || !json.Valid(r.Item)) {
		return errors.New("invalid completion item")
	}
	return nil
}
func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

// The application receives only this client. Docker access exists solely in analyzerd.
type Client struct{ http *http.Client }

func NewClient(socket string) *Client {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &Client{http: &http.Client{Transport: tr}}
}
func (c *Client) Assist(ctx context.Context, r AssistRequest) (AssistResponse, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return AssistResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://analyzers/assist", bytes.NewReader(body))
	if err != nil {
		return AssistResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return AssistResponse{}, fmt.Errorf("%w: service connection failed", ErrUnavailable)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 429:
		return AssistResponse{}, ErrBusy
	case 409:
		return AssistResponse{}, ErrStale
	case 200:
	default:
		return AssistResponse{}, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMessageBytes+1))
	if err != nil || len(data) > maxMessageBytes {
		return AssistResponse{}, ErrUnavailable
	}
	var result AssistResponse
	if json.Unmarshal(data, &result) != nil {
		return AssistResponse{}, ErrUnavailable
	}
	return result, nil
}

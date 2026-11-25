package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}
type session struct {
	cmd                *exec.Cmd
	name               string
	docker             string
	cancel             context.CancelFunc
	ctx                context.Context
	stdin              io.WriteCloser
	writeMu            sync.Mutex
	mu                 sync.Mutex
	pending            map[int64]chan rpcMessage
	sequence           atomic.Int64
	diagnostics        json.RawMessage
	diagVersion        *int
	diagSerial         int64
	changed            chan struct{}
	source             string
	revision           int64
	documentVersion    int
	uri                string
	language           string
	gate               chan struct{}
	last               atomic.Int64
	closeOnce          sync.Once
	done               chan struct{}
	capabilities       map[string]json.RawMessage
	resolveCompletion  bool
	pullDiagnostics    bool
	cleanupErr         error
	completionReady    bool
	completionDeadline time.Time
}

func (s *session) close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.stdin.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Explicit removal, independent of the canceled docker-attach process.
		if err := exec.CommandContext(ctx, s.docker, "rm", "-f", s.name).Run(); err != nil {
			out, checkErr := exec.CommandContext(ctx, s.docker, "ps", "-aq", "--filter", "name=^/"+s.name+"$").Output()
			if checkErr != nil || len(strings.TrimSpace(string(out))) > 0 {
				s.cleanupErr = fmt.Errorf("container cleanup failed: %s", s.name)
			}
		}
		select {
		case <-s.done:
		case <-ctx.Done():
		}
	})
	return s.cleanupErr
}
func (s *session) send(m rpcMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return WriteMessage(s.stdin, raw(m))
}
func (s *session) notify(method string, p any) error {
	return s.send(rpcMessage{JSONRPC: "2.0", Method: method, Params: raw(p)})
}
func (s *session) call(ctx context.Context, method string, p any) (json.RawMessage, error) {
	id := s.sequence.Add(1)
	ch := make(chan rpcMessage, 1)
	s.mu.Lock()
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
	if err := s.send(rpcMessage{JSONRPC: "2.0", ID: raw(id), Method: method, Params: raw(p)}); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		if len(m.Error) > 0 && string(m.Error) != "null" {
			return nil, errors.New("language server rejected request")
		}
		return m.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, ErrUnavailable
	}
}
func (s *session) read(stdout io.Reader) {
	defer s.cancel()
	r := bufio.NewReader(stdout)
	for {
		data, err := ReadMessage(r)
		if err != nil {
			return
		}
		var m rpcMessage
		if json.Unmarshal(data, &m) != nil {
			return
		}
		if m.Method != "" && len(m.ID) > 0 {
			result := any(nil)
			switch m.Method {
			case "workspace/configuration":
				var p struct{ Items []struct{ Section string } }
				if json.Unmarshal(m.Params, &p) != nil || len(p.Items) > 32 {
					return
				}
				settings := settings(s.language)
				values := make([]any, len(p.Items))
				for i, item := range p.Items {
					if item.Section == "" {
						values[i] = settings
					} else {
						values[i] = settings[item.Section]
					}
				}
				result = values
			case "workspace/workspaceFolders":
				result = []any{map[string]any{"uri": s.workspaceURI(), "name": "solution"}}
			case "workspace/applyEdit":
				result = map[string]any{"applied": false, "failureReason": "Only editor-side text edits are allowed"}
			}
			if s.send(rpcMessage{JSONRPC: "2.0", ID: m.ID, Result: raw(result)}) != nil {
				return
			}
			continue
		}
		if m.Method == "textDocument/publishDiagnostics" {
			var p struct {
				URI         string
				Version     *int
				Diagnostics json.RawMessage
			}
			if json.Unmarshal(m.Params, &p) != nil || p.URI != s.uri {
				continue
			}
			s.mu.Lock()
			s.diagnostics = p.Diagnostics
			s.diagVersion = p.Version
			s.diagSerial++
			close(s.changed)
			s.changed = make(chan struct{})
			s.mu.Unlock()
			continue
		}
		if len(m.ID) > 0 {
			var id int64
			if json.Unmarshal(m.ID, &id) != nil {
				continue
			}
			s.mu.Lock()
			ch := s.pending[id]
			s.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m:
				default:
				}
			}
		}
	}
}
func settings(language string) map[string]any {
	switch language {
	case "go":
		return map[string]any{"gopls": map[string]any{"completeUnimported": true, "usePlaceholders": true, "staticcheck": false}}
	case "python":
		return map[string]any{"rope": map[string]any{"ropeFolder": ".ropeproject"}, "pylsp": map[string]any{"plugins": map[string]any{"rope_autoimport": map[string]any{"enabled": true, "memory": false}, "pycodestyle": map[string]any{"enabled": false}, "pyflakes": map[string]any{"enabled": true}}}}
	}
	return map[string]any{}
}
func (s *session) initialize(ctx context.Context) error {
	result, err := s.call(ctx, "initialize", map[string]any{
		"processId": nil, "rootUri": s.workspaceURI(), "workspaceFolders": []any{map[string]any{"uri": s.workspaceURI(), "name": "solution"}},
		"capabilities": map[string]any{"workspace": map[string]any{"configuration": true}, "textDocument": map[string]any{
			"completion":         map[string]any{"completionItem": map[string]any{"snippetSupport": true, "resolveSupport": map[string]any{"properties": []string{"documentation", "detail", "additionalTextEdits"}}}},
			"publishDiagnostics": map[string]any{"versionSupport": true},
			"codeAction":         map[string]any{"codeActionLiteralSupport": map[string]any{"codeActionKind": map[string]any{"valueSet": []string{"quickfix", "source.organizeImports"}}}, "resolveSupport": map[string]any{"properties": []string{"edit"}}, "dataSupport": true},
		}}, "initializationOptions": settings(s.language),
	})
	if err != nil {
		return err
	}
	var initialization struct{ Capabilities map[string]json.RawMessage }
	if json.Unmarshal(result, &initialization) != nil {
		return ErrUnavailable
	}
	s.capabilities = initialization.Capabilities
	var completions struct{ ResolveProvider bool }
	_ = json.Unmarshal(s.capabilities["completionProvider"], &completions)
	s.resolveCompletion = completions.ResolveProvider
	diagnosticCapability := s.capabilities["diagnosticProvider"]
	s.pullDiagnostics = len(diagnosticCapability) > 0 && string(diagnosticCapability) != "false" && string(diagnosticCapability) != "null"
	if err = s.notify("initialized", map[string]any{}); err != nil {
		return err
	}
	return s.notify("workspace/didChangeConfiguration", map[string]any{"settings": settings(s.language)})
}
func (s *session) assist(ctx context.Context, r AssistRequest) (AssistResponse, error) {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return AssistResponse{}, ctx.Err()
	}
	s.last.Store(time.Now().UnixNano())
	if r.Revision < s.revision || (r.Revision == s.revision && r.Source != s.source) {
		return AssistResponse{}, ErrStale
	}
	changed := s.documentVersion == 0 || r.Source != s.source
	s.mu.Lock()
	serial := s.diagSerial
	s.mu.Unlock()
	if changed {
		s.mu.Lock()
		s.diagnostics = nil
		s.diagVersion = nil
		s.mu.Unlock()
	}
	if s.documentVersion == 0 {
		s.completionDeadline = time.Now().Add(15 * time.Second)
		s.documentVersion = 1
		if err := s.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": s.uri, "languageId": s.language, "version": s.documentVersion, "text": r.Source}}); err != nil {
			return AssistResponse{}, err
		}
	} else if changed {
		s.documentVersion++
		if err := s.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": s.uri, "version": s.documentVersion}, "contentChanges": []any{map[string]any{"text": r.Source}}}); err != nil {
			return AssistResponse{}, err
		}
	}
	s.source = r.Source
	s.revision = r.Revision
	response := AssistResponse{Revision: r.Revision, Result: raw(nil)}
	if r.Action == "diagnostics" {
		if s.pullDiagnostics {
			value, err := s.call(ctx, "textDocument/diagnostic", map[string]any{"textDocument": map[string]any{"uri": s.uri}})
			if err != nil {
				return response, err
			}
			var report struct{ Items json.RawMessage }
			if json.Unmarshal(value, &report) != nil {
				return response, ErrUnavailable
			}
			response.Result = report.Items
			if len(response.Result) == 0 {
				response.Result = raw([]any{})
			}
			s.mu.Lock()
			s.diagnostics = response.Result
			version := s.documentVersion
			s.diagVersion = &version
			s.mu.Unlock()
			return response, nil
		}
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for {
			s.mu.Lock()
			items, version, generation, ch := s.diagnostics, s.diagVersion, s.diagSerial, s.changed
			s.mu.Unlock()
			if items != nil && ((version != nil && *version == s.documentVersion) || (version == nil && (!changed || generation > serial))) {
				response.Result = items
				return response, nil
			}
			select {
			case <-ch:
			case <-timer.C:
				response.Pending = true
				return response, nil
			case <-ctx.Done():
				return response, ctx.Err()
			case <-s.ctx.Done():
				return response, ErrUnavailable
			}
		}
	}
	params := map[string]any{"textDocument": map[string]any{"uri": s.uri}, "position": r.Position}
	method := ""
	switch r.Action {
	case "completion":
		method = "textDocument/completion"
		params["context"] = map[string]any{"triggerKind": 1}
	case "hover":
		method = "textDocument/hover"
	case "signature":
		method = "textDocument/signatureHelp"
	case "format":
		method = "textDocument/formatting"
		delete(params, "position")
		params["options"] = map[string]any{"tabSize": 4, "insertSpaces": true}
	case "imports":
		method = "textDocument/codeAction"
		delete(params, "position")
		params["range"] = map[string]any{"start": Position{}, "end": Position{}}
		params["context"] = map[string]any{"diagnostics": []any{}, "only": []string{"source.organizeImports"}}
	case "fixes":
		method = "textDocument/codeAction"
		delete(params, "position")
		params["range"] = map[string]any{"start": r.Position, "end": r.Position}
		s.mu.Lock()
		diagnostics := s.diagnostics
		s.mu.Unlock()
		if len(diagnostics) == 0 {
			diagnostics = raw([]any{})
		}
		params["context"] = map[string]any{"diagnostics": diagnostics, "only": []string{"quickfix"}}
	case "resolveAction":
		method = "codeAction/resolve"
	case "resolveCompletion":
		if !s.resolveCompletion {
			response.Result = r.Item
			return response, nil
		}
		method = "completionItem/resolve"
	default:
		return response, fmt.Errorf("invalid action")
	}
	var value any = params
	if r.Action == "resolveCompletion" || r.Action == "resolveAction" {
		value = r.Item
	}
	result, err := s.call(ctx, method, value)
	response.Result = result
	if err == nil && s.language == "csharp" && r.Action == "completion" {
		response.Pending = s.pendingCompletion(result, time.Now())
	}
	return response, err
}

// Fresh Roslyn workspaces can return null before their project is loaded.
// Retry only within a bounded cold-start window, never for empty results forever.
func (s *session) pendingCompletion(result json.RawMessage, now time.Time) bool {
	s.mu.Lock()
	hasDiagnostics := s.diagnostics != nil
	s.mu.Unlock()
	var items []json.RawMessage
	empty := false
	if json.Unmarshal(result, &items) == nil {
		empty = len(items) == 0
	} else {
		var list struct{ Items []json.RawMessage }
		empty = json.Unmarshal(result, &list) == nil && len(list.Items) == 0
	}
	if !empty || hasDiagnostics {
		s.completionReady = true
	}
	return empty && !s.completionReady && now.Before(s.completionDeadline)
}

func (s *session) workspaceURI() string { return s.uri[:strings.LastIndex(s.uri, "/")] }

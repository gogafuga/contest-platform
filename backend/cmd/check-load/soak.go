package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"devsec-contest01/internal/analyzer"
	"devsec-contest01/internal/tasks"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errActiveRuns = errors.New("soak still has unfinished runs")

type soakStat struct {
	Count    int   `json:"count"`
	Failures int   `json:"failures"`
	P50      int64 `json:"p50Ms"`
	P95      int64 `json:"p95Ms"`
	Max      int64 `json:"maxMs"`
}
type soakSample struct {
	ms     int64
	failed bool
}
type soakMetrics struct {
	mu       sync.Mutex
	started  time.Time
	samples  map[string][]soakSample
	minutes  map[int]map[string][]soakSample
	counters map[string]int
	errors   []string
}

func percentile(values []int64, percent int) int64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]int64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return copyValues[(percent*len(copyValues)+99)/100-1]
}
func summarize(samples []soakSample) soakStat {
	var values []int64
	result := soakStat{Count: len(samples)}
	for _, s := range samples {
		if s.failed {
			result.Failures++
		}
		values = append(values, s.ms)
	}
	result.P50 = percentile(values, 50)
	result.P95 = percentile(values, 95)
	result.Max = percentile(values, 100)
	return result
}
func (m *soakMetrics) record(action string, started time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := soakSample{time.Since(started).Milliseconds(), err != nil}
	m.samples[action] = append(m.samples[action], s)
	minute := int(time.Since(m.started) / time.Minute)
	if m.minutes[minute] == nil {
		m.minutes[minute] = make(map[string][]soakSample)
	}
	m.minutes[minute][action] = append(m.minutes[minute][action], s)
	if err != nil && len(m.errors) < 30 {
		m.errors = append(m.errors, fmt.Sprintf("%s: %v", action, err))
	}
}
func (m *soakMetrics) increment(name string) { m.mu.Lock(); m.counters[name]++; m.mu.Unlock() }
func (m *soakMetrics) snapshot() (map[string]soakStat, map[int]map[string]soakStat, map[string]int, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := make(map[string]soakStat)
	minutes := make(map[int]map[string]soakStat)
	counts := make(map[string]int)
	for k, v := range m.samples {
		total[k] = summarize(v)
	}
	for n, actions := range m.minutes {
		minutes[n] = make(map[string]soakStat)
		for k, v := range actions {
			minutes[n][k] = summarize(v)
		}
	}
	for k, v := range m.counters {
		counts[k] = v
	}
	return total, minutes, counts, append([]string(nil), m.errors...)
}

type soakStudent struct {
	base         *student
	f            fixture
	program      string
	needle, want string
	mu           sync.RWMutex
	version      int64
	source       string
}

func (s *soakStudent) current() (int64, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version, s.source
}
func (s *soakStudent) save(a api, source string) error {
	version, _ := s.current()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var row struct {
		Version    int64
		SourceCode string
	}
	_, err := a.request(ctx, "PUT", taskPath+"/draft", s.base.Token, map[string]any{"version": version, "sourceCode": source, "language": s.f.language}, &row)
	if err == nil && (row.Version != version+1 || row.SourceCode != source) {
		err = fmt.Errorf("save content/version mismatch for student %d", s.base.Number)
	}
	if err != nil {
		// Recover the known version after an ambiguous response, without hiding
		// the failed operation in metrics or overwriting newer data blindly.
		if _, getErr := a.request(ctx, "GET", taskPath+"/draft", s.base.Token, nil, &row); getErr != nil {
			return err
		}
	}
	s.mu.Lock()
	s.version, s.source = row.Version, row.SourceCode
	s.mu.Unlock()
	return err
}
func soakProgram(language string) (string, string, string) {
	switch language {
	case "go":
		return "package main\nimport (\"fmt\";\"os\")\nfunc main(){\n data,err:=os.ReadFile(\"input.txt\");if err!=nil{panic(err)}\n stdout,err:=os.Create(\"stdout\");if err!=nil{panic(err)};defer stdout.Close()\n fmt.Fprintln(stdout,len(data))\n}\n", "fmt.Fprintln", "Fprintln"
	case "python":
		return "import os\nwith open(\"input.txt\",encoding=\"utf-8\") as source:\n    data=source.read()\nwith open(\"stdout\",\"w\",encoding=\"utf-8\") as stdout:\n    stdout.write(str(len(data))+\"\\n\")\npath=os.path.basename(\"input.txt\")\n", "os.path", "path"
	default:
		return "using System;\nusing System.IO;\nclass Program { static void Main() {\n var data=File.ReadAllText(\"input.txt\");\n using var stdout=new StreamWriter(\"stdout\");stdout.WriteLine(data.Length);\n}}\n", "File.ReadAllText", "ReadAllText"
	}
}
func completionPosition(source, needle string) (int, int, error) {
	i := strings.Index(source, needle)
	if i < 0 {
		return 0, 0, fmt.Errorf("fixture completion marker not found")
	}
	prefix := source[:i+len(needle)]
	return strings.Count(prefix, "\n"), len(prefix) - strings.LastIndex(prefix, "\n") - 1, nil
}
func (s *soakStudent) hint(ctx context.Context, a api, m *soakMetrics) error {
	for ctx.Err() == nil {
		version, source := s.current()
		line, column, err := completionPosition(source, s.needle)
		if err != nil {
			return err
		}
		var response struct {
			Result  json.RawMessage
			Pending bool
			Queued  bool
		}
		status, err := a.request(ctx, "POST", taskPath+"/assist", s.base.Token, map[string]any{"clientId": s.base.ClientID, "language": s.f.language, "source": source, "revision": version, "action": "completion", "position": map[string]int{"line": line, "character": column}}, &response)
		if status == 429 {
			m.increment("hintBusy")
			if err = pause(ctx, 2*time.Second); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if response.Pending {
			m.increment("hintPending")
			delay := time.Second
			if response.Queued {
				delay = 250 * time.Millisecond
			}
			if err = pause(ctx, delay); err != nil {
				return err
			}
			continue
		}
		latest, _ := s.current()
		if latest != version {
			m.increment("staleHintReplies")
			continue
		}
		var list struct{ Items []struct{ Label string } }
		if json.Unmarshal(response.Result, &list) != nil {
			_ = json.Unmarshal(response.Result, &list.Items)
		}
		for _, item := range list.Items {
			if strings.HasPrefix(item.Label, s.want) {
				return nil
			}
		}
		return fmt.Errorf("student %d/%s missing completion %s", s.base.Number, s.f.language, s.want)
	}
	return ctx.Err()
}
func soakRun(a api, s *soakStudent, expected string, m *soakMetrics) error {
	_, source := s.current()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	var row struct {
		ID, Status, Stdout, Feedback string
		Passed                       bool
		Earned                       float64
	}
	status, err := a.request(ctx, "POST", taskPath+"/runs", s.base.Token, map[string]string{"requestId": uuid(), "language": s.f.language, "sourceCode": source}, &row)
	if status == 429 {
		m.increment("runRejected")
	}
	if err != nil {
		return err
	}
	for row.Status == "queued" || row.Status == "running" {
		if err = pause(ctx, 500*time.Millisecond); err != nil {
			return err
		}
		if _, err = a.request(ctx, "GET", taskPath+"/runs/"+row.ID, s.base.Token, nil, &row); err != nil {
			return err
		}
	}
	if row.Status != "wrong_answer" || row.Passed || row.Earned != 0 || strings.TrimSpace(row.Stdout) != expected {
		return fmt.Errorf("student %d unexpected intermediate result %s: %.100s", s.base.Number, row.Status, row.Feedback)
	}
	return nil
}
func soak(a api, pool *pgxpool.Pool, count int, duration time.Duration, reportPath string, capacity *analyzer.Capacity) error {
	if capacity == nil || capacity.AdmissionBlocked {
		return fmt.Errorf("soak requires a running updated analyzer service with /status")
	}
	people := make([]*soakStudent, count)
	for i := range people {
		f := fixtures[i%len(fixtures)]
		program, needle, want := soakProgram(f.language)
		s := &soakStudent{base: &student{Number: i + 1, Language: f.language, ClientID: randomID()}, f: f, program: program, needle: needle, want: want}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		var registered struct{ Token string }
		_, err := a.request(ctx, "POST", "/api/v1/register", "", map[string]string{"name": fmt.Sprintf("Soak student %d", i+1), "group": "Soak test"}, &registered)
		s.base.Token = registered.Token
		if err == nil {
			_, err = a.request(ctx, "POST", taskPath+"/select", s.base.Token, nil, nil)
		}
		cancel()
		if err != nil {
			return err
		}
		if err = s.save(a, program); err != nil {
			return err
		}
		people[i] = s
	}
	task, _ := tasks.NewRegistry().Get("cidr-check")
	expected := strconv.Itoa(len(task.Dataset))
	started := time.Now()
	m := &soakMetrics{started: started, samples: make(map[string][]soakSample), minutes: make(map[int]map[string][]soakSample), counters: make(map[string]int)}
	signalCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()
	work, cancelWork := context.WithTimeout(signalCtx, duration)
	defer cancelWork()
	fmt.Printf("Смешанный прогон: %d участников · %s · сохранение каждые 2с, подсказки каждые 4–8с после ответа, запуск каждые 60с после результата.\n", count, duration)
	var wg sync.WaitGroup
	for i, s := range people {
		wg.Add(3)
		go func(s *soakStudent) {
			defer wg.Done()
			edit := 0
			for pause(work, 2*time.Second) == nil {
				edit++
				comment := "//"
				if s.f.language == "python" {
					comment = "#"
				}
				source := fmt.Sprintf("%s%s edit %d\n", s.program, comment, edit)
				begin := time.Now()
				err := s.save(a, source)
				m.record("save", begin, err)
				if edit%5 == 0 {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					var row struct {
						Version    int64
						SourceCode string
					}
					begin = time.Now()
					_, err = a.request(ctx, "GET", taskPath+"/draft", s.base.Token, nil, &row)
					cancel()
					version, current := s.current()
					if err == nil && (row.Version != version || row.SourceCode != current) {
						err = fmt.Errorf("student %d restored source differs", s.base.Number)
					}
					m.record("restore", begin, err)
				}
			}
		}(s)
		go func(i int, s *soakStudent) {
			defer wg.Done()
			if pause(work, time.Duration(i)*100*time.Millisecond) != nil {
				return
			}
			for work.Err() == nil {
				begin := time.Now()
				ctx, cancel := context.WithTimeout(work, 90*time.Second)
				err := s.hint(ctx, a, m)
				cancel()
				if work.Err() != nil {
					break
				}
				m.record("hint", begin, err)
				if pause(work, time.Duration(4+i%5)*time.Second) != nil {
					break
				}
			}
		}(i, s)
		go func(i int, s *soakStudent) {
			defer wg.Done()
			if pause(work, time.Duration(i)*time.Second) != nil {
				return
			}
			for work.Err() == nil {
				begin := time.Now()
				err := soakRun(a, s, expected, m)
				m.record("run", begin, err)
				if pause(work, time.Minute) != nil {
					break
				}
			}
		}(i, s)
	}
	type observation struct {
		ElapsedMS  int64             `json:"elapsedMs"`
		Capacity   analyzer.Capacity `json:"capacity"`
		Goroutines int               `json:"testProcessGoroutines"`
		HeapMiB    uint64            `json:"testProcessHeapMiB"`
	}
	var observations []observation
	tick := time.NewTicker(time.Minute)
	for work.Err() == nil {
		select {
		case <-work.Done():
		case <-tick.C:
			total, _, _, _ := m.snapshot()
			fmt.Printf("[%s] сохранений %d, подсказок %d, запусков %d · p95 подсказок %d мс · ошибок %d\n", time.Since(started).Round(time.Second), total["save"].Count, total["hint"].Count, total["run"].Count, total["hint"].P95, total["save"].Failures+total["hint"].Failures+total["run"].Failures)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			c, err := analyzer.NewClient("/run/contest-analyzers/lsp.sock").Capacity(ctx)
			cancel()
			if err != nil {
				m.increment("statusErrors")
			} else {
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				observations = append(observations, observation{time.Since(started).Milliseconds(), c, runtime.NumGoroutine(), stats.HeapAlloc >> 20})
				if c.AdmissionBlocked || c.LiveSessions > capacity.MaxSessions {
					m.record("capacity", time.Now(), fmt.Errorf("analyzer admission/capacity unhealthy"))
					cancelWork()
				}
			}
		}
	}
	tick.Stop()
	workElapsed := time.Since(started)
	fmt.Println("Рабочий интервал завершён; ожидаю текущие сохранения и запуски…")
	wg.Wait()
	for _, s := range people {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := a.request(ctx, "POST", taskPath+"/assist", s.base.Token, map[string]any{"clientId": s.base.ClientID, "language": s.f.language, "action": "close", "source": "", "revision": 1, "position": map[string]int{"line": 0, "character": 0}}, nil)
		cancel()
		if err != nil {
			m.record("close", time.Now(), err)
		}
	}
	if signalCtx.Err() == nil && workElapsed >= duration-time.Second {
		fmt.Println("Финальная проверка правильных решений и баллов…")
		for _, s := range people {
			wg.Add(1)
			go func(s *soakStudent) {
				defer wg.Done()
				begin := time.Now()
				err := s.save(a, s.f.source)
				m.record("finalSave", begin, err)
				if err != nil {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()
				begin = time.Now()
				err = execute(ctx, a, s.base, s.f)
				m.record("finalRun", begin, err)
			}(s)
		}
		wg.Wait()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var history, versions, accepted, broken, pending int64
	err := pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM draft_history),(SELECT COALESCE(SUM(version),0)::bigint FROM drafts),(SELECT COUNT(*) FROM submissions WHERE score=150),
        (SELECT COUNT(*) FROM (SELECT d.participant_id FROM drafts d LEFT JOIN draft_history h USING(participant_id,task_id) GROUP BY d.participant_id,d.task_id,d.version HAVING COUNT(h.version)<>d.version OR MIN(h.version)<>1 OR MAX(h.version)<>d.version) gaps)`).Scan(&history, &versions, &accepted, &broken)
	if err != nil {
		m.record("integrity", time.Now(), err)
	} else if history != versions || broken != 0 {
		m.record("integrity", time.Now(), fmt.Errorf("history=%d versions=%d broken=%d", history, versions, broken))
	}
	total, minutes, counters, errors := m.snapshot()
	if err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM code_runs WHERE status IN ('queued','running')").Scan(&pending); err != nil {
		return fmt.Errorf("%w: cannot verify queue drained", errActiveRuns)
	}
	failures := 0
	for _, s := range total {
		failures += s.Failures
	}
	interrupted := signalCtx.Err() != nil || workElapsed < duration-time.Second
	report := map[string]any{"mode": "soak", "participants": count, "requestedDurationMs": duration.Milliseconds(), "workElapsedMs": workElapsed.Milliseconds(), "totalElapsedMs": time.Since(started).Milliseconds(), "interrupted": interrupted, "stats": total, "minutes": minutes, "counters": counters, "errors": errors, "observations": observations, "historySnapshots": history, "sumDraftVersions": versions, "historyBrokenParticipants": broken, "finalAccepted": accepted, "analyzerCapacity": capacity, "scope": "temporary database, loopback API and real isolated supervisors; no browser/Wi-Fi simulation; source edits are comment-only; hint staleness retried"}
	if err = os.MkdirAll(filepath.Dir(reportPath), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(reportPath, append(data, '\n'), 0600); err != nil {
		return err
	}
	fmt.Printf("Прогон завершён: ошибок %d · история %d/%d · правильных решений %d/%d · отчёт %s\n", failures, history, versions, accepted, count, reportPath)
	if pending > 0 {
		return fmt.Errorf("%w: %d", errActiveRuns, pending)
	}
	if failures > 0 || interrupted || accepted != int64(count) {
		return fmt.Errorf("soak incomplete or has failures; inspect report")
	}
	return nil
}

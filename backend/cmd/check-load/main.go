// A bounded rehearsal against a disposable database and loopback-only API.
// Fixed source fixtures are compiled/analyzed ONLY by the sandbox supervisors.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"devsec-contest01/internal/analyzer"
	"devsec-contest01/internal/config"
	"devsec-contest01/internal/server"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const taskPath = "/api/v1/tasks/cidr-check"

var retainAssist bool
var skipAssist bool
var soakDuration time.Duration

type fixture struct {
	language, source, completion, want string
	line, column                       int
}

var fixtures = []fixture{
	{"go", `package main
import("bufio";"fmt";"os";"net/netip")
func main(){f,e:=os.Open("input.txt");if e!=nil{panic(e)};defer f.Close();o,e:=os.Create("stdout");if e!=nil{panic(e)};defer o.Close();p:=netip.MustParsePrefix("172.16.32.0/20");seen:=map[string]bool{};s:=bufio.NewScanner(f);for s.Scan(){v:=s.Text();a,e:=netip.ParseAddr(v);if e==nil&&a.Is4()&&!p.Contains(a)&&!seen[v]{seen[v]=true;fmt.Fprintln(o,v)}};if e:=s.Err();e!=nil{panic(e)}}`, "package main\nimport \"fmt\"\nfunc main(){fmt.Pr}\n", "Println", 2, 18},
	{"python", `import ipaddress
allowed = ipaddress.ip_network("172.16.32.0/20")
seen = set()
with open("input.txt") as source, open("stdout", "w") as output:
    for line in source:
        value = line.strip()
        if value and value not in seen and ipaddress.ip_address(value) not in allowed:
            seen.add(value)
            print(value, file=output)
`, "import os\nos.pa\n", "path", 1, 5},
	{"csharp", `using System;using System.IO;using System.Net;using System.Collections.Generic;
class Program{static void Main(){using var output=new StreamWriter("stdout");var seen=new HashSet<string>();foreach(var line in File.ReadLines("input.txt")){if(!IPAddress.TryParse(line,out var ip))continue;var b=ip.GetAddressBytes();if(b.Length==4&&!(b[0]==172&&b[1]==16&&b[2]>=32&&b[2]<=47)&&seen.Add(line))output.WriteLine(line);}}}`, "using System;\nclass P { static void Main(){Console.Wri} }", "WriteLine", 1, 40},
}

type student struct {
	Number         int            `json:"number"`
	Language       string         `json:"language"`
	Token          string         `json:"-"`
	ClientID       string         `json:"-"`
	Errors         []string       `json:"errors"`
	SaveMS         int64          `json:"saveMs"`
	HintColdMS     int64          `json:"hintColdMs"`
	HintWarmMS     int64          `json:"hintWarmMs"`
	HintQueueMS    int64          `json:"hintQueueMs"`
	HintStartupMS  int64          `json:"hintStartupMs"`
	HintAnalysisMS int64          `json:"hintAnalysisMs"`
	BusyResponses  int            `json:"busyResponses"`
	BusyReasons    map[string]int `json:"busyReasons,omitempty"`
	RunRejected    bool           `json:"runRejected"`
	RunMS          int64          `json:"runTotalMs"`
	CompileRunMS   int64          `json:"compileRunMs"`
	Earned         float64        `json:"earned"`
}

type api struct {
	url    string
	client *http.Client
}

func (a api) request(ctx context.Context, method, path, token string, body any, output any) (int, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, a.url+path, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	data, err = io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return response.StatusCode, err
	}
	if response.StatusCode >= 400 {
		return response.StatusCode, fmt.Errorf("HTTP %d: %.250s", response.StatusCode, data)
	}
	if output != nil && len(data) > 0 {
		err = json.Unmarshal(data, output)
	}
	return response.StatusCode, err
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func uuid() string {
	s := randomID()
	return s[:8] + "-" + s[8:12] + "-4" + s[13:16] + "-a" + s[17:20] + "-" + s[20:]
}
func pause(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func save(ctx context.Context, a api, s *student, f fixture) error {
	var registered struct{ Token string }
	if _, err := a.request(ctx, "POST", "/api/v1/register", "", map[string]string{"name": fmt.Sprintf("Load student %d", s.Number), "group": "Load test"}, &registered); err != nil {
		return err
	}
	s.Token = registered.Token
	if _, err := a.request(ctx, "POST", taskPath+"/select", s.Token, nil, nil); err != nil {
		return err
	}
	started := time.Now()
	var draft struct {
		Version    int64
		SourceCode string
	}
	var source string
	for version := int64(0); version < 3; version++ {
		comment := "//"
		if f.language == "python" {
			comment = "#"
		}
		source = fmt.Sprintf("%s saved revision %d\n%s", comment, version+1, f.source)
		if _, err := a.request(ctx, "PUT", taskPath+"/draft", s.Token, map[string]any{"version": version, "sourceCode": source, "language": f.language}, &draft); err != nil {
			return err
		}
		if draft.Version != version+1 {
			return fmt.Errorf("wrong saved version %d", draft.Version)
		}
	}
	if _, err := a.request(ctx, "GET", taskPath+"/draft", s.Token, nil, &draft); err != nil {
		return err
	}
	if draft.Version != 3 || draft.SourceCode != source {
		return fmt.Errorf("saved code was not restored")
	}
	status, _ := a.request(ctx, "PUT", taskPath+"/draft", s.Token, map[string]any{"version": 1, "sourceCode": "stale", "language": f.language}, nil)
	if status != 409 {
		return fmt.Errorf("stale save: want 409, got %d", status)
	}
	s.SaveMS = time.Since(started).Milliseconds()
	return nil
}

func hints(ctx context.Context, a api, s *student, f fixture) error {
	startedHint := time.Now()
	defer func() {
		if s.HintColdMS == 0 {
			s.HintColdMS = time.Since(startedHint).Milliseconds()
		}
	}()
	clientID := randomID()
	s.ClientID = clientID
	payload := map[string]any{"clientId": clientID, "language": f.language, "source": f.completion, "revision": 1, "action": "completion", "position": map[string]int{"line": f.line, "character": f.column}}
	defer func() {
		if retainAssist {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		payload["action"] = "close"
		_, _ = a.request(closeCtx, "POST", taskPath+"/assist", s.Token, payload, nil)
	}()
	for attempt := 0; attempt < 2; attempt++ {
		started := time.Now()
		for {
			var response struct {
				Result                         json.RawMessage
				Pending                        bool
				Queued                         bool
				QueueMS, StartupMS, AnalysisMS int64
			}
			status, err := a.request(ctx, "POST", taskPath+"/assist", s.Token, payload, &response)
			if status == 429 {
				s.BusyResponses++
				if s.BusyReasons == nil {
					s.BusyReasons = make(map[string]int)
				}
				s.BusyReasons[err.Error()]++
				if err = pause(ctx, 2*time.Second); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if response.Pending {
				delay := time.Second
				if response.Queued {
					delay = 250 * time.Millisecond
				}
				if err = pause(ctx, delay); err != nil {
					return err
				}
				continue
			}
			var list struct{ Items []struct{ Label string } }
			if json.Unmarshal(response.Result, &list) != nil {
				_ = json.Unmarshal(response.Result, &list.Items)
			}
			found := false
			for _, item := range list.Items {
				if strings.HasPrefix(item.Label, f.want) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("completion %s not found", f.want)
			}
			if attempt == 0 {
				s.HintQueueMS, s.HintStartupMS, s.HintAnalysisMS = response.QueueMS, response.StartupMS, response.AnalysisMS
			}
			break
		}
		elapsed := time.Since(started).Milliseconds()
		if attempt == 0 {
			s.HintColdMS = elapsed
		} else {
			s.HintWarmMS = elapsed
		}
	}
	return nil
}

func execute(ctx context.Context, a api, s *student, f fixture) error {
	started := time.Now()
	var run struct {
		ID, Status, Feedback string
		Passed               bool
		Earned               float64
		DurationMS           int64
	}
	if status, err := a.request(ctx, "POST", taskPath+"/runs", s.Token, map[string]string{"requestId": uuid(), "language": f.language, "sourceCode": f.source}, &run); err != nil {
		s.RunRejected = status == 429
		return err
	}
	for run.Status == "queued" || run.Status == "running" {
		if err := pause(ctx, 250*time.Millisecond); err != nil {
			return err
		}
		if _, err := a.request(ctx, "GET", taskPath+"/runs/"+run.ID, s.Token, nil, &run); err != nil {
			return err
		}
	}
	s.RunMS = time.Since(started).Milliseconds()
	s.CompileRunMS = run.DurationMS
	s.Earned = run.Earned
	if run.Status != "accepted" || !run.Passed || run.Earned != 150 {
		return fmt.Errorf("run %s: %s", run.Status, run.Feedback)
	}
	var state struct{ Scores map[string]float64 }
	if _, err := a.request(ctx, "GET", "/api/v1/me", s.Token, nil, &state); err != nil {
		return err
	}
	if state.Scores["cidr-check"] != 150 {
		return fmt.Errorf("score not persisted")
	}
	return nil
}

func phase(name string, students []*student, a api, action func(context.Context, api, *student, fixture) error) {
	fmt.Println(name + "…")
	var wg sync.WaitGroup
	for i, s := range students {
		wg.Add(1)
		go func(i int, s *student) {
			defer wg.Done()
			if len(s.Errors) > 0 && s.Token == "" {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
			defer cancel()
			if err := action(ctx, a, s, fixtures[i%len(fixtures)]); err != nil {
				s.Errors = append(s.Errors, name+": "+err.Error())
			}
		}(i, s)
	}
	wg.Wait()
}

func run(count int, dsn, reportPath string) error {
	ctx := context.Background()
	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	parsed.ConnConfig.Database = "postgres" // Never use the application DB for test rows.
	admin, err := pgxpool.NewWithConfig(ctx, parsed)
	if err != nil {
		return fmt.Errorf("maintenance connection: %w", err)
	}
	defer admin.Close()
	name := "contest_load_" + randomID()
	identifier := pgx.Identifier{name}.Sanitize()
	keepDatabase := false
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		return fmt.Errorf("create test database: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if keepDatabase {
			cancel()
			fmt.Fprintln(os.Stderr, "Сохранена тестовая база с незавершёнными запусками:", name)
			return
		}
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			fmt.Fprintln(os.Stderr, "Не удалена временная база:", name, err)
		} else {
			fmt.Println("Временная база удалена.")
		}
	}()
	testConfig := parsed.Copy()
	testConfig.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		return err
	}
	defer pool.Close()
	paths, err := filepath.Glob("db/migrations/*.sql")
	if err != nil || len(paths) == 0 {
		return fmt.Errorf("run from backend directory (db/migrations not found)")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err = pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("migration %s: %w", path, err)
		}
	}
	// Distinct analyzer owners avoid collisions with real participants on shared supervisors.
	if _, err = pool.Exec(ctx, "SELECT setval('participants_id_seq', $1)", time.Now().UnixMilli()*1000); err != nil {
		return err
	}
	gin.SetMode(gin.ReleaseMode)
	router, err := server.New(ctx, config.Config{AdminToken: randomID(), SessionTTLHours: 1, ContestDurationSeconds: 5400}, pool)
	if err != nil {
		return err
	}
	apiServer := httptest.NewServer(router)
	defer apiServer.Close()
	a := api{apiServer.URL, &http.Client{Timeout: 55 * time.Second}}
	var analyzerCapacity *analyzer.Capacity
	if !skipAssist {
		statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		capacity, err := analyzer.NewClient("/run/contest-analyzers/lsp.sock").Capacity(statusCtx)
		cancel()
		if err == nil {
			analyzerCapacity = &capacity
			fmt.Printf("Анализаторы: %d контейнеров · HTTP-мест %d · ожидающих %d\n", capacity.MaxSessions, capacity.RequestSlots, capacity.Waiting)
			if capacity.AdmissionBlocked {
				return fmt.Errorf("analyzer supervisor has disabled admission; restart it before the test")
			}
		} else {
			fmt.Println("Сервис анализаторов не сообщает лимиты: перезапустите обновлённую версию для проверки конфигурации.")
		}
	}
	students := make([]*student, count)
	if soakDuration > 0 {
		err := soak(a, pool, count, soakDuration, reportPath, analyzerCapacity)
		keepDatabase = errors.Is(err, errActiveRuns)
		return err
	}
	for i := range students {
		students[i] = &student{Number: i + 1, Language: fixtures[i%len(fixtures)].language, Errors: []string{}}
	}
	started := time.Now()
	phase("Сохранение и восстановление", students, a, save)
	if !skipAssist {
		phase("Подсказки: первый и повторный запрос", students, a, hints)
	} else {
		fmt.Println("Подсказки пропущены: проверяются только сохранения и запуски.")
	}
	if retainAssist {
		// Keep successful editors open while others wait: tests reclamation,
		// rather than freeing every slot explicitly as soon as it responds.
		for _, s := range students {
			if s.ClientID == "" {
				continue
			}
			closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := a.request(closeCtx, "POST", taskPath+"/assist", s.Token, map[string]any{"clientId": s.ClientID, "language": s.Language, "source": "", "revision": 1, "action": "close", "position": map[string]int{"line": 0, "character": 0}}, nil)
			cancel()
			if err != nil {
				s.Errors = append(s.Errors, "Закрытие анализатора: "+err.Error())
			}
		}
	}
	phase("Выполнение и начисление баллов", students, a, execute)
	var historyCount, acceptedCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM draft_history").Scan(&historyCount); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM submissions WHERE score = 150").Scan(&acceptedCount); err != nil {
		return err
	}
	failed, busy, rejected := 0, 0, 0
	for _, s := range students {
		busy += s.BusyResponses
		if s.RunRejected {
			rejected++
		}
		hintTiming := fmt.Sprintf("подсказки %d/%d мс", s.HintColdMS, s.HintWarmMS)
		if skipAssist {
			hintTiming = "подсказки пропущены"
		}
		fmt.Printf("Участник %d · %s · сохранение %d мс · %s · запуск %d мс (код %d мс) · баллы %.0f · ошибок %d\n", s.Number, s.Language, s.SaveMS, hintTiming, s.RunMS, s.CompileRunMS, s.Earned, len(s.Errors))
		if !skipAssist {
			fmt.Printf("  Первые подсказки: очередь %d · подготовка %d · анализ %d мс\n", s.HintQueueMS, s.HintStartupMS, s.HintAnalysisMS)
		}
		for _, message := range s.Errors {
			fmt.Println("  " + message)
		}
		if len(s.Errors) > 0 {
			failed++
		}
	}
	var runs []int64
	for _, s := range students {
		if s.RunMS > 0 {
			runs = append(runs, s.RunMS)
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
	var p95 int64
	if len(runs) > 0 {
		p95 = runs[(95*len(runs)+99)/100-1]
	}
	report := map[string]any{"createdAt": time.Now().UTC(), "participants": students, "elapsedMs": time.Since(started).Milliseconds(), "failedParticipants": failed, "analyzerBusyResponses": busy, "runP95Ms": p95, "historySnapshots": historyCount, "acceptedSubmissions": acceptedCount, "scope": "temporary database and loopback API; shared real sandbox supervisors; no browser or Wi-Fi simulation"}
	report["runQueueRejected"] = rejected
	report["hintsSkipped"] = skipAssist
	report["analyzerCapacity"] = analyzerCapacity
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(reportPath), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(reportPath, append(data, '\n'), 0600); err != nil {
		return err
	}
	fmt.Printf("Итого: %d участников, с ошибками %d, ответов «анализаторы заняты» %d, p95 запуска %d мс.\nОтчёт: %s\n", count, failed, busy, p95, reportPath)
	fmt.Printf("История: %d/%d версий · успешных решений: %d/%d · отклонено запусков очередью: %d\n", historyCount, count*3, acceptedCount, count, rejected)
	if historyCount != count*3 || acceptedCount != count {
		return fmt.Errorf("database verification: snapshots=%d (want %d), accepted=%d (want %d)", historyCount, count*3, acceptedCount, count)
	}
	if failed > 0 {
		return fmt.Errorf("load check has failures")
	}
	return nil
}

func main() {
	count := flag.Int("participants", 5, "simultaneous participants (1–25)")
	stages := flag.Bool("stages", false, "run 10, 20 and 25 participants sequentially, preserving each report even on test failures")
	report := flag.String("report", "../work/load-test/report.json", "JSON report without tokens")
	flag.BoolVar(&retainAssist, "retain-assist", false, "keep editor sessions open until every participant has received completions")
	flag.BoolVar(&skipAssist, "skip-assist", false, "test saves and execution only; report explicitly marks hints as skipped")
	flag.DurationVar(&soakDuration, "soak", 0, "mixed sustained workload, e.g. 10m (30s–30m)")
	flag.Parse()
	if soakDuration != 0 {
		if soakDuration < 30*time.Second || soakDuration > 30*time.Minute || *stages || skipAssist {
			fmt.Fprintln(os.Stderr, "soak requires 30s–30m and cannot combine with stages/skip-assist")
			os.Exit(2)
		}
		reportSet := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "report" {
				reportSet = true
			}
		})
		if !reportSet {
			*report = "../work/load-test/soak-report.json"
		}
	}
	if *count < 1 || *count > 25 {
		fmt.Fprintln(os.Stderr, "participants must be 1–25")
		os.Exit(2)
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://contest:contest@127.0.0.1:55432/postgres?sslmode=disable"
	}
	if *stages {
		failed := false
		for _, n := range []int{10, 20, 25} {
			ext := filepath.Ext(*report)
			path := strings.TrimSuffix(*report, ext) + fmt.Sprintf("-%d", n) + ext
			fmt.Printf("\nПроверка %d участников\n", n)
			if err := run(n, dsn, path); err != nil {
				fmt.Fprintf(os.Stderr, "Ступень %d: %v\n", n, err)
				failed = true
			}
		}
		if failed {
			os.Exit(1)
		}
		return
	}
	if err := run(*count, dsn, *report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

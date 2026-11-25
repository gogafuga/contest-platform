//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRegistrationAndAuthentication(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.json("GET", "/health", "", "", 200)
	for _, body := range []string{`{`, `{}`, `{"name":"","group":"CS"}`, `{"name":"Alice","group":" "}`} {
		f.json("POST", "/api/v1/register", "", body, 400)
	}
	token := f.register("Alice Student")
	var sessionLifetimeSeconds float64
	if err := f.pool.QueryRow(context.Background(), "SELECT EXTRACT(EPOCH FROM (expires_at - created_at)) FROM sessions").Scan(&sessionLifetimeSeconds); err != nil {
		t.Fatal(err)
	}
	if sessionLifetimeSeconds < 336*3600-5 || sessionLifetimeSeconds > 336*3600+5 {
		t.Fatalf("session lifetime = %f seconds, want 14 days", sessionLifetimeSeconds)
	}
	state := f.me(token)
	if state.Participant.Name != "Alice Student" || state.Participant.Group != "CS-101" || state.SelectedTaskID != nil || len(state.Scores) != 0 {
		t.Fatalf("unexpected initial state: %+v", state)
	}
	f.json("POST", "/api/v1/register", "", `{"name":"  ALICE STUDENT  ","group":"Other"}`, 409)
	for _, invalid := range []string{"", "invalid-token", adminToken} {
		for _, route := range []struct{ method, path string }{
			{
				method: "GET",
				path:   "/api/v1/me",
			},
			{
				method: "GET",
				path:   "/api/v1/tasks/cidr-check/input",
			},
			{
				method: "POST",
				path:   "/api/v1/tasks/cidr-check/select",
			},
			{
				method: "POST",
				path:   "/api/v1/tasks/cidr-check/submit",
			},
		} {
			f.json(route.method, route.path, invalid, "", 401)
		}
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE sessions SET expires_at = NOW() - INTERVAL '1 second'"); err != nil {
		t.Fatal(err)
	}
	f.json("GET", "/api/v1/me", token, "", 401)
}

func TestContestProgressionAndPersistence(t *testing.T) {
	t.Parallel()
	f := setup(t)
	token := f.register("Alice Student")
	other := f.register("Bob Student")
	cases := []struct {
		id     string
		pkg    string
		points float64
	}{
		{
			id:     "noisy-ips",
			pkg:    "task_01",
			points: 125,
		},
		{
			id:     "password-validator",
			pkg:    "task_02",
			points: 100,
		},
		{
			id:     "cidr-check",
			pkg:    "task_03",
			points: 150,
		},
	}
	// Discover metadata through HTTP; ensure it doesn't serialize datasets/answers.
	metadata := decode[[]map[string]json.RawMessage](t, f.json("GET", "/api/v1/tasks", "", "", 200))
	if len(metadata) != 3 {
		t.Fatalf("got %d task descriptions", len(metadata))
	}
	for _, task := range metadata {
		if len(task) != 8 {
			t.Fatalf("unexpected metadata fields: %v", task)
		}
		for _, key := range []string{"dataset", "expected", "validate"} {
			if _, ok := task[key]; ok {
				t.Fatalf("metadata exposes %s", key)
			}
		}
	}
	for _, task := range cases {
		f.json("GET", "/api/v1/tasks/"+task.id+"/input", token, "", 409)
		f.upload(task.id, token, []byte("wrong"), 409)
	}
	f.selectTask("unknown-task", token, 404)
	f.json("GET", "/api/v1/tasks/unknown-task/input", token, "", 404)
	f.upload("unknown-task", token, []byte("wrong"), 404)

	scores := map[string]float64{}
	for i, task := range cases {
		f.selectTask(task.id, token, 200)
		f.selectTask(task.id, token, 200) // Selecting the active task is idempotent.
		next := cases[(i+1)%len(cases)]
		f.selectTask(next.id, token, 409)
		downloaded := f.json("GET", "/api/v1/tasks/"+task.id+"/input", token, "", 200)
		if !bytes.Equal(downloaded.body, readTaskFile(t, task.pkg, "dataset.txt")) {
			t.Fatal("download differs from committed dataset")
		}
		if !strings.Contains(downloaded.header.Get("Content-Disposition"), task.id+".txt") {
			t.Fatal("download filename missing")
		}
		for attempt := 0; attempt < 2; attempt++ {
			failed := decode[verdict](t, f.upload(task.id, token, []byte("wrong-output\n"), 200))
			if failed.Passed || failed.Earned != 0 || failed.TaskID != task.id {
				t.Fatalf("unexpected failure: %+v", failed)
			}
		}
		scores[task.id] = 0
		if state := f.me(token); !reflect.DeepEqual(state.Scores, scores) {
			t.Fatalf("failed scores: %v, want %v", state.Scores, scores)
		}
		f.selectTask(next.id, token, 409)

		// Exercise multipart judging with a reordered, duplicated, CRLF answer.
		lines := strings.Split(strings.TrimSuffix(string(readTaskFile(t, task.pkg, "expected.txt")), "\n"), "\n")
		slices.Reverse(lines)
		lines = append(lines, lines[0])
		passed := decode[verdict](t, f.upload(task.id, token, []byte(strings.Join(lines, "\r\n")+"\r\n"), 200))
		if !passed.Passed || passed.Earned != task.points || passed.TaskID != task.id {
			t.Fatalf("unexpected pass: %+v", passed)
		}
		scores[task.id] = task.points
		f.upload(task.id, token, []byte("wrong"), 409)
		f.upload(task.id, token, readTaskFile(t, task.pkg, "expected.txt"), 409)
		if state := f.me(token); state.SelectedTaskID == nil || *state.SelectedTaskID != task.id || !reflect.DeepEqual(state.Scores, scores) {
			t.Fatalf("unexpected persisted state: %+v", state)
		}
	}
	// Switching away must not remove access to an already solved task.
	f.json("GET", "/api/v1/tasks/"+cases[0].id+"/input", token, "", 200)
	f.upload(cases[0].id, token, []byte("wrong"), 409)
	if state := f.me(other); state.SelectedTaskID != nil || len(state.Scores) != 0 {
		t.Fatalf("participant state leaked: %+v", state)
	}
	f.selectTask(cases[0].id, other, 200)
	f.upload(cases[0].id, other, []byte("wrong"), 200)
	f.selectTask(cases[1].id, other, 409)

	type entry struct {
		Name   string
		Group  string
		Scores map[string]float64
		Total  float64
	}
	board := decode[struct{ Entries []entry }](t, f.json("GET", "/api/v1/leaderboard", "", "", 200))
	if len(board.Entries) != 2 || board.Entries[0].Name != "Alice Student" || board.Entries[0].Group != "CS-101" || board.Entries[0].Total != 375 || !reflect.DeepEqual(board.Entries[0].Scores, scores) || board.Entries[1].Name != "Bob Student" || board.Entries[1].Total != 0 {
		t.Fatalf("unexpected leaderboard: %+v", board.Entries)
	}
	// A fresh router/services instance must restore selections, scores and sessions.
	f.startServer()
	state := f.me(token)
	if state.SelectedTaskID == nil || *state.SelectedTaskID != "cidr-check" || !reflect.DeepEqual(state.Scores, scores) {
		t.Fatalf("state lost after restart: %+v", state)
	}
}

func TestUploadErrors(t *testing.T) {
	t.Parallel()
	f := setup(t)
	token := f.register("Upload Student")
	f.selectTask("password-validator", token, 200)
	f.json("POST", "/api/v1/tasks/password-validator/submit", token, "{}", 400)
	f.upload("password-validator", token, bytes.Repeat([]byte("x"), (2<<20)+1), 413)
	// Neither request may create a score or complete/lock out the selected task.
	if state := f.me(token); len(state.Scores) != 0 {
		t.Fatalf("invalid uploads changed scores: %v", state.Scores)
	}
	passed := decode[verdict](t, f.upload("password-validator", token, readTaskFile(t, "task_02", "expected.txt"), 200))
	if !passed.Passed || passed.Earned != 100 {
		t.Fatalf("valid retry failed: %+v", passed)
	}
}

func TestLeaderboardRanking(t *testing.T) {
	t.Parallel()
	f := setup(t)
	bob := f.register("Bob Student")
	alice := f.register("Alice Student")
	type entry struct {
		Name  string
		Total float64
	}
	board := func() []entry {
		return decode[struct{ Entries []entry }](t, f.json("GET", "/api/v1/leaderboard", "", "", 200)).Entries
	}
	// Equal scores have a deterministic alphabetical tie-break.
	entries := board()
	if len(entries) != 2 || entries[0].Name != "Alice Student" || entries[1].Name != "Bob Student" {
		t.Fatalf("unexpected tie ordering: %+v", entries)
	}
	f.selectTask("cidr-check", bob, 200)
	f.upload("cidr-check", bob, readTaskFile(t, "task_03", "expected.txt"), 200)
	f.selectTask("password-validator", alice, 200)
	f.upload("password-validator", alice, readTaskFile(t, "task_02", "expected.txt"), 200)
	entries = board()
	if entries[0].Name != "Bob Student" || entries[0].Total != 150 || entries[1].Name != "Alice Student" || entries[1].Total != 100 {
		t.Fatalf("unexpected ranking: %+v", entries)
	}
}

type clock struct {
	StartedAt       *time.Time
	DurationSeconds int
	Status          string
}

func TestClock(t *testing.T) {
	t.Parallel()
	f := setup(t)
	get := func() clock { return decode[clock](t, f.json("GET", "/api/v1/clock", "", "", 200)) }
	initial := get()
	if initial.StartedAt != nil || initial.DurationSeconds != 5400 || initial.Status != "not_started" {
		t.Fatalf("unexpected initial clock: %+v", initial)
	}
	for _, token := range []string{"", "wrong-token", f.register("Clock Student")} {
		f.json("POST", "/api/v1/admin/clock/start", token, `{"durationSeconds":120}`, 401)
	}
	if !reflect.DeepEqual(get(), initial) {
		t.Fatal("unauthorized request changed clock")
	}
	started := decode[clock](t, f.json("POST", "/api/v1/admin/clock/start", adminToken, `{"durationSeconds":120}`, 200))
	if started.StartedAt == nil || started.DurationSeconds != 120 || started.Status != "running" {
		t.Fatalf("unexpected started clock: %+v", started)
	}
	if !reflect.DeepEqual(get(), started) {
		t.Fatal("public clock differs from admin response")
	}
	for _, body := range []string{`{`, `{"durationSeconds":"bad"}`, `{"durationSeconds":59}`, `{"durationSeconds":1209601}`} {
		f.json("POST", "/api/v1/admin/clock/start", adminToken, body, 400)
		if !reflect.DeepEqual(get(), started) {
			t.Fatal("invalid request changed clock")
		}
	}
	f.cfg.ContestDurationSeconds = 900 // New config must not overwrite persisted duration.
	f.startServer()
	if !reflect.DeepEqual(get(), started) {
		t.Fatal("server restart reset clock")
	}
	// Move time in the isolated database instead of waiting for the contest to end.
	if _, err := f.pool.Exec(context.Background(), "UPDATE contest_state SET started_at = NOW() - INTERVAL '10 minutes'"); err != nil {
		t.Fatal(err)
	}
	ended := get()
	if ended.Status != "ended" {
		t.Fatalf("unexpected ended clock: %+v", ended)
	}
	restarted := decode[clock](t, f.request("POST", "/api/v1/admin/clock/start", adminToken, "", nil, 200))
	if restarted.DurationSeconds != 120 || restarted.Status != "running" || restarted.StartedAt == nil || !restarted.StartedAt.After(*ended.StartedAt) {
		t.Fatalf("unexpected restart: %+v", restarted)
	}
	for _, duration := range []int{60, 86400, 86401, 604800, 1209600} {
		result := decode[clock](t, f.json("POST", "/api/v1/admin/clock/start", adminToken, fmt.Sprintf(`{"durationSeconds":%d}`, duration), 200))
		if result.DurationSeconds != duration {
			t.Fatalf("duration %d not persisted", duration)
		}
	}
}

//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"devsec-contest01/internal/service"
)

func TestTaskTimingSelectionAndCompletion(t *testing.T) {
	f := setup(t)
	token := f.register("Timing student")
	id := f.me(token).Participant.ID
	path := fmt.Sprintf("/api/v1/admin/participants/%d/tasks/noisy-ips/timing", id)
	f.json("GET", path, "", "", 401)
	f.json("GET", path, token, "", 401)
	read := func() service.TaskTiming {
		return decode[service.TaskTiming](t, f.json("GET", path, adminToken, "", 200))
	}
	if read().StartedAt != nil {
		t.Fatal("unselected task has a start time")
	}
	f.selectTask("noisy-ips", token, 200)
	first := read()
	if first.StartedAt == nil || first.Estimated || first.FinishedAt != nil {
		t.Fatalf("unexpected selected timing: %+v", first)
	}
	f.selectTask("noisy-ips", token, 200)
	if !read().StartedAt.Equal(*first.StartedAt) {
		t.Fatal("reselection reset task start")
	}
	f.selectTask("password-validator", token, 409)
	if _, err := f.pool.Exec(context.Background(), "UPDATE participant_task_timing SET started_at = NOW() - INTERVAL '10 minutes' WHERE participant_id=$1", id); err != nil {
		t.Fatal(err)
	}
	f.upload("noisy-ips", token, []byte("wrong"), 200)
	ongoing := read()
	if ongoing.FinishedAt != nil || ongoing.ElapsedSeconds < 600 {
		t.Fatalf("failed submission stopped timing: %+v", ongoing)
	}
	f.upload("noisy-ips", token, readTaskFile(t, "task_01", "expected.txt"), 200)
	solved := read()
	if solved.FinishedAt == nil || solved.ElapsedSeconds < 600 {
		t.Fatalf("success did not finish timing: %+v", solved)
	}
	f.selectTask("password-validator", token, 200)
	f.selectTask("noisy-ips", token, 409) // Still cannot abandon an unsolved task.
	if later := read(); later.ElapsedSeconds != solved.ElapsedSeconds || !later.FinishedAt.Equal(*solved.FinishedAt) {
		t.Fatal("solved task's duration changed")
	}
}

func TestTaskTimingLegacyBackfill(t *testing.T) {
	f := setup(t)
	token := f.register("Legacy timing student")
	id := f.me(token).Participant.ID
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO draft_history (participant_id, task_id, source_code, version, created_at)
	VALUES ($1, 'noisy-ips', '', 1, $2), ($1, 'noisy-ips', '', 2, $2::timestamptz + INTERVAL '5 minutes')`, id, start)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join(backendRoot(), "db/migrations/007_task_timing.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = f.pool.Exec(context.Background(), string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	f.selectTask("noisy-ips", token, 200)
	path := fmt.Sprintf("/api/v1/admin/participants/%d/tasks/noisy-ips/timing", id)
	timing := decode[service.TaskTiming](t, f.json("GET", path, adminToken, "", 200))
	if !timing.Estimated || timing.StartedAt == nil || !timing.StartedAt.Equal(start) || timing.ElapsedSeconds < 3600 {
		t.Fatalf("legacy start not preserved: %+v", timing)
	}
}

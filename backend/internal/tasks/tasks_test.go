package tasks

import (
	"os"
	"strings"
	"testing"
)

func TestTasksAcceptEmptyOutputLines(t *testing.T) {
	r := NewRegistry()
	for _, tc := range []struct {
		id   string
		path string
	}{
		{"noisy-ips", "task_01/data/expected.txt"},
		{"password-validator", "task_02/data/expected.txt"},
		{"cidr-check", "task_03/data/expected.txt"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			expected, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			task, ok := r.Get(tc.id)
			if !ok {
				t.Fatal("task missing from registry")
			}
			output := "\n\n" + strings.ReplaceAll(string(expected), "\n", "\n\n") + "\n\n"
			for _, newline := range []string{"\n", "\r\n"} {
				if got := task.Validate([]byte(strings.ReplaceAll(output, "\n", newline))); !got.Passed {
					t.Fatalf("correct answer with empty lines (%q): %+v", newline, got)
				}
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	ids := []string{"noisy-ips", "password-validator", "cidr-check"}
	list := r.List()
	if len(list) != len(ids) {
		t.Fatalf("List() returned %d tasks, want %d", len(list), len(ids))
	}
	for i, want := range ids {
		if list[i].ID != want {
			t.Fatalf("List()[%d].ID = %q, want %q", i, list[i].ID, want)
		}
	}

	for _, id := range ids {
		task, ok := r.Get(id)
		if !ok {
			t.Fatalf("Get(%q) not found", id)
		}
		if task.Points <= 0 {
			t.Fatalf("task %q has non-positive points", id)
		}
		if len(task.Dataset) == 0 {
			t.Fatalf("task %q has empty dataset", id)
		}
		if task.Code == "" || task.Title == "" || task.Brief == "" {
			t.Fatalf("task %q has empty description fields", id)
		}
		if len(task.Rules) == 0 {
			t.Fatalf("task %q has no rules", id)
		}
		if task.SampleInput == "" || task.SampleOutput == "" {
			t.Fatalf("task %q has empty sample dataset/output", id)
		}
	}

	if _, ok := r.Get("does-not-exist"); ok {
		t.Fatalf("Get() unexpectedly found an unknown task id")
	}
}

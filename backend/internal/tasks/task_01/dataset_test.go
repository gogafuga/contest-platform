package task_01

import (
	"slices"
	"strings"
	"testing"
)

// Check that the stored answer stays in sync with the reference solver.
func TestDatasetAnswer(t *testing.T) {
	got := strings.Join(Solve(dataset), "\n") + "\n"
	if got != expected {
		t.Fatal("committed answer disagrees with reference solver; inspect dataset or solver")
	}
	task := New()
	if !task.Validate([]byte(expected)).Passed {
		t.Fatal("committed answer rejected")
	}
	lines := strings.Split(strings.TrimSuffix(expected, "\n"), "\n")
	slices.Reverse(lines)
	if !task.Validate([]byte(strings.Join(lines, "\n"))).Passed {
		t.Fatal("reordered correct output rejected")
	}
	if !task.Validate([]byte(expected + expected)).Passed {
		t.Fatal("duplicate matching values rejected")
	}
	if task.Validate(append([]byte(expected), []byte("extra-line\n")...)).Passed {
		t.Fatal("extra output accepted")
	}
	if task.Validate([]byte(task.SampleOutput)).Passed {
		t.Fatal("public sample must not solve held-out dataset")
	}
}

func TestSampleAnswer(t *testing.T) {
	got := strings.Join(Solve([]byte(sampleInput)), "\n")
	if got != sampleOutput {
		t.Fatalf("sample answer mismatch: got %q, want %q", got, sampleOutput)
	}
}

// Keep the generated files in sync with their task-owned generator.
func TestGeneratedDataset(t *testing.T) {
	generatedDataset := Generate()
	if strings.Join(generatedDataset, "\n")+"\n" != string(dataset) {
		t.Fatal("generated dataset differs from embedded file; regenerate datasets")
	}
}

// Command generate-datasets writes the datasets constructed by each task.
// Run from backend: go run ./tools/generate-datasets
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devsec-contest01/internal/tasks/task_01"
	"devsec-contest01/internal/tasks/task_02"
	"devsec-contest01/internal/tasks/task_03"
)

func main() {
	if _, err := os.Stat("internal/tasks"); err != nil {
		panic("run from backend directory")
	}
	for _, task := range []struct {
		directory string
		generate  func() []string
		solve     func([]byte) []string
	}{
		{"task_01", task_01.Generate, task_01.Solve},
		{"task_02", task_02.Generate, task_02.Solve},
		{"task_03", task_03.Generate, task_03.Solve},
	} {
		dataset := task.generate()
		answer := task.solve([]byte(strings.Join(dataset, "\n") + "\n"))
		directory := filepath.Join("internal/tasks", task.directory, "data")
		if err := os.MkdirAll(directory, 0755); err != nil {
			panic(err)
		}
		for _, file := range []struct {
			name  string
			lines []string
		}{
			{"dataset.txt", dataset}, {"expected.txt", answer},
		} {
			if err := os.WriteFile(filepath.Join(directory, file.name), []byte(strings.Join(file.lines, "\n")+"\n"), 0644); err != nil {
				panic(err)
			}
		}
		fmt.Printf("%s: %d dataset lines, %d answer lines\n", task.directory, len(dataset), len(answer))
	}
}

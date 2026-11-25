package task_01

import (
	"devsec-contest01/internal/tasks/model"
	_ "embed"
	"regexp"
	"sort"
	"strings"

	"devsec-contest01/internal/tasks/textutil"
)

// threshold is fixed with the dataset. Regenerate data after changing it.
const threshold = 5

//go:embed data/dataset.txt
var dataset []byte

//go:embed data/expected.txt
var expected string

var accessLogLineRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[(\d{2}/\w+/\d{4}:\d{2}:\d{2}:\d{2})`)

func New() *model.Task {
	return &model.Task{
		Description: description(),
		Dataset:     dataset,
		Validate:    model.MatchingValues(strings.Split(strings.TrimSuffix(expected, "\n"), "\n")),
	}
}

// Solve produces the expected output using this task’s threshold.
func Solve(dataset []byte) []string {
	return solve(dataset, threshold)
}

// solve flags every IP whose event count within the same to-the-second
// timestamp bucket reaches threshold. Sorting makes the stored answer deterministic.
func solve(dataset []byte, threshold int) []string {
	counts := make(map[string]int) // "ip|timestamp" -> count in that second
	maxPerIP := make(map[string]int)

	for _, line := range textutil.SplitNonBlank(string(dataset)) {
		m := accessLogLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ip, ts := m[1], m[2]

		key := ip + "|" + ts
		counts[key]++
		if counts[key] > maxPerIP[ip] {
			maxPerIP[ip] = counts[key]
		}
	}

	result := make([]string, 0)
	for ip := range maxPerIP {
		if maxPerIP[ip] >= threshold {
			result = append(result, ip)
		}
	}
	sort.Strings(result)
	return result
}

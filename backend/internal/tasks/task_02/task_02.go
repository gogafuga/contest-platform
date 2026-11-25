package task_02

import (
	"devsec-contest01/internal/tasks/model"
	_ "embed"
	"strings"

	"devsec-contest01/internal/tasks/textutil"
)

//go:embed data/dataset.txt
var dataset []byte

//go:embed data/expected.txt
var expected string

const symbols = "!@#$%^&*()-_=+"

func New() *model.Task {
	return &model.Task{
		Description: description(),
		Dataset:     dataset,
		Validate:    model.MatchingValues(strings.Split(strings.TrimSuffix(expected, "\n"), "\n")),
	}
}

// Solve returns unique passwords that violate at least one requirement. Blank lines are skipped.
func Solve(dataset []byte) []string {
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, line := range textutil.SplitLines(string(dataset)) {
		if line == "" {
			continue
		}
		if !seen[line] && !isValid(line) {
			seen[line] = true
			result = append(result, line)
		}
	}
	return result
}

func isValid(pw string) bool {
	if len(pw) < 8 || len(pw) > 64 {
		return false
	}
	var hasUpper, hasLower, hasDigit, hasSymbol, hasSpace bool
	for _, r := range pw {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case strings.ContainsRune(symbols, r):
			hasSymbol = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f':
			hasSpace = true
		}
	}
	return hasUpper && hasLower && hasDigit && hasSymbol && !hasSpace
}

// Package model defines task data and output validation shared by task implementations.
package model

import (
	"slices"
	"strings"
)

type Description struct {
	ID           string   `json:"id"`
	Code         string   `json:"code"`
	Title        string   `json:"title"`
	Points       float64  `json:"points"`
	Brief        string   `json:"brief"`
	Rules        []string `json:"rules"`
	SampleInput  string   `json:"sampleInput"`
	SampleOutput string   `json:"sampleOutput"`
}

type ValidationResult struct {
	Passed   bool
	Feedback string
}

type Task struct {
	Description
	Dataset  []byte
	Validate func([]byte) ValidationResult
}

// MatchingValues compares sets of output lines: order, duplicates and empty lines
// are ignored. Spaces, missing values and unexpected values still matter.
// CRLF is accepted. Copy expected before sorting it.
func MatchingValues(expected []string) func([]byte) ValidationResult {
	answer := slices.Clone(expected)
	slices.Sort(answer)
	answer = slices.Compact(answer)
	return func(submitted []byte) ValidationResult {
		output := strings.ReplaceAll(string(submitted), "\r\n", "\n")
		var lines []string
		for _, line := range strings.Split(output, "\n") {
			if line != "" {
				lines = append(lines, line)
			}
		}
		slices.Sort(lines)
		lines = slices.Compact(lines)
		if slices.Equal(lines, answer) {
			return ValidationResult{
				Passed:   true,
				Feedback: "Решение принято",
			}
		}
		return ValidationResult{
			Feedback: "Неверный вывод. Проверьте, что указаны все подходящие значения и нет лишних.",
		}
	}
}

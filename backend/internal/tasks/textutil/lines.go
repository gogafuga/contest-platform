// Package textutil holds the tiny line-splitting helpers shared by judge.go
// and every per-task package. It has to be a leaf package with no dependency
// on judge (which imports the per-task packages, so a dependency back on
// judge would be an import cycle).
package textutil

import "strings"

// SplitNonBlank splits on newlines, trims each line, and drops blank ones —
// used for flat lists of answers.
func SplitNonBlank(s string) []string {
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))
	for _, l := range raw {
		if trimmed := strings.TrimSpace(l); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// SplitLines trims only the trailing \r left by CRLF line endings, preserving
// blank lines and order — needed by tasks that parse dataset positionally
// rather than treating it as a flat list of answers.
func SplitLines(s string) []string {
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))
	for _, l := range raw {
		lines = append(lines, strings.TrimRight(l, "\r"))
	}
	return lines
}

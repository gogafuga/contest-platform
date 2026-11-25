package task_03

import (
	"devsec-contest01/internal/tasks/model"
	_ "embed"
	"net/netip"
	"strings"

	"devsec-contest01/internal/tasks/textutil"
)

//go:embed data/dataset.txt
var dataset []byte

//go:embed data/expected.txt
var expected string

func New() *model.Task {
	return &model.Task{
		Description: description(),
		Dataset:     dataset,
		Validate:    model.MatchingValues(strings.Split(strings.TrimSuffix(expected, "\n"), "\n")),
	}
}

// Solve returns unique IPv4 addresses outside the fixed allowed subnet.
func Solve(dataset []byte) []string {
	prefix := netip.MustParsePrefix(allowedCIDR)
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, raw := range textutil.SplitLines(string(dataset)) {
		ip, err := netip.ParseAddr(raw)
		if err != nil || !ip.Is4() {
			continue
		}
		if !prefix.Contains(ip) && !seen[raw] {
			seen[raw] = true
			result = append(result, raw)
		}
	}
	return result
}

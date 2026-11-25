package service

import "testing"

func TestNeedsSolvedCheck(t *testing.T) {
	cases := []struct {
		name    string
		current string
		taskID  string
		want    bool
	}{
		{"nothing selected yet", "", "task01", false},
		{"re-selecting the active task", "task01", "task01", false},
		{"switching to a different task", "task01", "task02", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsSolvedCheck(tc.current, tc.taskID); got != tc.want {
				t.Fatalf("needsSolvedCheck(%q, %q) = %v, want %v", tc.current, tc.taskID, got, tc.want)
			}
		})
	}
}

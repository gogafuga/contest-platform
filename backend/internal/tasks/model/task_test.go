package model

import "testing"

func TestMatchingValues(t *testing.T) {
	validate := MatchingValues([]string{"a", "b"})
	for _, tc := range []struct {
		name, output string
		passed       bool
	}{
		{"exact", "a\nb", true},
		{"final newline", "a\nb\n", true},
		{"windows", "a\r\nb\r\n", true},
		{"partial", "a", false},
		{"wrong", "a\nc", false},
		{"order ignored", "b\na", true},
		{"extra line", "a\nb\nc", false},
		{"extra blank", "a\nb\n\n", true},
		{"interior blank", "a\n\nb", true},
		{"leading blanks", "\n\na\nb", true},
		{"windows blanks", "\r\na\r\n\r\nb\r\n\r\n", true},
		{"only blanks", "\n\n", false},
		{"whitespace line", "a\n \t\nb", false},
		{"spaces", " a\nb", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := validate([]byte(tc.output))
			if got.Passed != tc.passed || got.Feedback == "" {
				t.Fatalf("got %+v, want passed=%v with feedback", got, tc.passed)
			}
		})
	}
}

func TestMatchingValuesDuplicates(t *testing.T) {
	expected := []string{"b", "a", "b"}
	validate := MatchingValues(expected)
	for _, tc := range []struct {
		output string
		passed bool
	}{
		{"b\nb\na", true},
		{"a", false},
		{"a\nb\nc", false},
		{"a\nb", true},
		{"a\nb\nb\nb", true},
		{"a\na\nb", true},
	} {
		if got := validate([]byte(tc.output)); got.Passed != tc.passed {
			t.Fatalf("%q: got %+v, want passed=%v", tc.output, got, tc.passed)
		}
	}
	if expected[0] != "b" {
		t.Fatal("validator mutated expected lines")
	}
}

package task_02

import (
	"reflect"
	"testing"
)

func TestIsValid(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		want bool
	}{
		{"valid", "Spender2026!", true},
		{"too short", "Ab1!", false},
		{"no digit", "NoDigitsHere!", false},
		{"has space", "HasSpace 123A!", false},
		{"no upper", "nouppercase1!", false},
		{"no lower", "NOLOWERCASE1!", false},
		{"no symbol", "NoSymbolHere1", false},
		{"length exactly 8 valid", "Abcdef1!", true},
		{"length 7 invalid", "Abcde1!", false},
		{"length exactly 64 valid", "Aa1!" + stringOfLen(60, 'x'), true},
		{"length 65 invalid", "Aa1!" + stringOfLen(61, 'x'), false},
		{"tab is whitespace", "Abc\t1234!", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isValid(c.pw); got != c.want {
				t.Fatalf("isValid(%q) = %v, want %v", c.pw, got, c.want)
			}
		})
	}
}

func TestSolve(t *testing.T) {
	dataset := []byte("Spender2026!\nAb1!\nNoDigitsHere!\nHasSpace 123A!")
	got := Solve(dataset)
	want := []string{"Ab1!", "NoDigitsHere!", "HasSpace 123A!"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSolveSkipsBlankLines(t *testing.T) {
	dataset := []byte("Spender2026!\n\nAb1!\n\n")
	got := Solve(dataset)
	if len(got) != 1 || got[0] != "Ab1!" {
		t.Fatalf("got %v, want [Ab1!]", got)
	}
}

func stringOfLen(n int, r byte) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = r
	}
	return string(b)
}

func TestSolveReturnsUniqueMatchingValues(t *testing.T) {
	got := Solve([]byte("Zz1!aaaa\nAb1!\nAa1!aaaa\nZz1!aaaa\nAb1!\n Aa1!aaaa\nAa1!aaaa\t\n"))
	want := []string{"Ab1!", " Aa1!aaaa", "Aa1!aaaa\t"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

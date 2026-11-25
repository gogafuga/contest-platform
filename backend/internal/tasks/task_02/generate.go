package task_02

import (
	"math/rand"
	"strings"
)

// Generate constructs only the fixed-seed dataset for this task.
func Generate() (dataset []string) {
	rng := rand.New(rand.NewSource(20261110))
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" + symbols
	rows := []string{
		"Aa1!aaaa",
		"Aa1!aaa",
		"Aa1!" + strings.Repeat("a", 60),
		"Aa1!" + strings.Repeat("a", 61),
		"Aa1! aaA",
		"\tAa1!aaaa",
		"Aa1!aaaa ",
		"Aa1!aaaa",
		"Aa1.aaaa",
	}
	for i := 0; i < 5000; i++ {
		size := []int{8, 9, 16, 32, 63, 64}[rng.Intn(6)]
		pw := []byte("Aa1!" + strings.Repeat("a", size-4))
		for j := 4; j < size; j++ {
			pw[j] = alphabet[rng.Intn(len(alphabet))]
		}
		valid := string(pw)
		kind := i % 10
		switch kind {
		case 1:
			valid = strings.ToLower(valid)
		case 2:
			valid = strings.ToUpper(valid)
		case 3:
			valid = strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return 'a'
				}
				return r
			}, valid)
		case 4:
			valid = strings.Map(func(r rune) rune {
				if strings.ContainsRune(symbols, r) {
					return 'a'
				}
				return r
			}, valid)
		case 5:
			pw[3] = ' '
			valid = string(pw)
		case 6:
			valid = valid[:7]
		case 7:
			valid += strings.Repeat("a", 65)
		case 8:
			pw[4] = '\t'
			valid = string(pw)
		}
		// Include missing classes, whitespace, and invalid lengths.
		rows = append(rows, valid)
	}
	rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return rows
}

package task_03

import (
	"reflect"
	"testing"
)

func TestSolve(t *testing.T) {
	cases := []struct {
		name    string
		dataset string
		want    []string
	}{
		{"sample", "172.16.32.10\n172.16.47.255\n172.16.48.1\n192.0.2.1", []string{"172.16.48.1", "192.0.2.1"}},
		{"boundaries", "172.16.31.255\n172.16.32.0\n172.16.32.1\n172.16.47.254\n172.16.47.255\n172.16.48.0", []string{"172.16.31.255", "172.16.48.0"}},
		{"all allowed", "172.16.33.1\n172.16.40.99\n", []string{}},
		{"unique and CRLF", "192.0.2.1\r\n0.0.0.0\r\n192.0.2.1\r\n255.255.255.255\r\n", []string{"192.0.2.1", "0.0.0.0", "255.255.255.255"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Solve([]byte(c.dataset)); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

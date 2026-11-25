package runner

import (
	"strings"
	"testing"
)

func TestRejectsInvalidExecutionRequests(t *testing.T) {
	good := Request{Language: "go", Source: "package main", Task: "noisy-ips", Input: []byte("input")}
	if good.Validate() != nil {
		t.Fatal("valid request rejected")
	}
	cases := []Request{good, good, good, good, good}
	cases[0].Language = "sh"
	cases[1].Source = ""
	cases[2].Task = "../../secret"
	cases[3].Source = strings.Repeat("x", MaxSource+1)
	cases[4].Input = make([]byte, MaxInput+1)
	for _, r := range cases {
		if r.Validate() == nil {
			t.Fatal("invalid execution accepted")
		}
	}
}
func TestRunnerHasNoWeakerRuntimeFallback(t *testing.T) {
	args := containerArgs("test", "contest-run-go:local", "go")
	found := false
	for _, arg := range args {
		if arg == "--runtime=runsc" {
			found = true
		}
		if strings.Contains(arg, "privileged") || strings.Contains(arg, "unconfined") || strings.Contains(arg, "docker.sock") {
			t.Fatalf("unsafe option: %s", arg)
		}
	}
	if !found {
		t.Fatal("gVisor must be mandatory")
	}
}

func TestLanguageFileLimits(t *testing.T) {
	for _, language := range []string{"go", "python", "csharp"} {
		args := containerArgs("test", "contest-run-"+language+":local", language)
		want := "--ulimit=fsize=33554432:33554432"
		if language == "csharp" {
			want = "--ulimit=fsize=134217728:134217728"
		}
		found := false
		for _, arg := range args {
			if arg == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: missing %s", language, want)
		}
	}
}

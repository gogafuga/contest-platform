// Smoke tests talk only to the private runner service. Never execute fixtures on the host.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"devsec-contest01/internal/runner"
)

func main() {
	client := runner.NewClient("/run/contest-runner/run.sock")
	fixtures := []struct{ language, source string }{
		{"go", "package main\nimport (\"fmt\";\"os\";\"io\")\nfunc main(){a,_:=os.ReadFile(\"input.txt\");b,_:=os.ReadFile(\"smoke.txt\");c,_:=io.ReadAll(os.Stdin);fmt.Printf(\"%s%s%s\",a,b,c)}"},
		{"python", "import sys\nprint(open('input.txt').read()+open('smoke.txt').read()+sys.stdin.read(),end='')"},
		{"csharp", "using System; using System.IO; class Program { static void Main() { Console.Write(File.ReadAllText(\"input.txt\")+File.ReadAllText(\"smoke.txt\")+Console.In.ReadToEnd()); }}"},
	}
	for _, f := range fixtures {
		check(client, f.language, f.source, "ok", "sample\nsample\nsample\n")
	}
	// Verify answer-file collection for each language, independent of console output.
	check(client, "go", "package main\nimport (\"os\";\"fmt\")\nfunc main(){stdout,err:=os.Create(\"stdout\");if err!=nil{panic(err)};defer stdout.Close();fmt.Fprintln(stdout,\"answer-file\")}", "ok", "answer-file\n")
	check(client, "python", "with open('stdout','w',encoding='utf-8') as stdout:\n    print('answer-file',file=stdout)", "ok", "answer-file\n")
	check(client, "csharp", "using System.IO; class Program { static void Main() { using var stdout = new StreamWriter(\"stdout\"); stdout.WriteLine(\"answer-file\"); }}", "ok", "answer-file\n")
	check(client, "csharp", "using System; class P { static void Main(){ Console.WriteLine(123) }}", "compile_error", "")
	check(client, "python", "def main(:", "compile_error", "")
	check(client, "go", "package main\nfunc main(){undefined()}", "compile_error", "")
	check(client, "python", "while True: pass", "time_limit", "")
	check(client, "python", "print('x'*1100000)", "output_limit", "")
	check(client, "python", "raise Exception('fixture')", "runtime_error", "")
	fmt.Println("Проверки runner завершены: Go, Python, C#, входные файлы, файл ответа stdout, stdin, ошибки и лимиты.")
}
func check(client *runner.Client, language, source, status, output string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	result, err := client.Run(ctx, runner.Request{Language: language, Source: source, Task: "smoke", Input: []byte("sample\n")})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Runner недоступен:", err)
		os.Exit(1)
	}
	if result.Status != status || (output != "" && string(result.Stdout) != output) {
		fmt.Fprintf(os.Stderr, "FAIL %s: %s (exit=%d)\nstdout:\n%s\nstderr:\n%s\n", language, result.Status, result.ExitCode, strings.TrimSpace(string(result.Stdout)), strings.TrimSpace(string(result.Stderr)))
		os.Exit(1)
	}
	fmt.Printf("%s: %s — успешно · компиляция+выполнение %d мс · всего %d мс\n", language, status, result.DurationMS, time.Since(started).Milliseconds())
}

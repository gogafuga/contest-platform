// Fixed fixtures sent to the private analyzer service; never analyze user code on the host.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"devsec-contest01/internal/analyzer"
)

func main() {
	socket := flag.String("socket", "/run/contest-analyzers/lsp.sock", "private analyzer socket")
	flag.Parse()
	client := analyzer.NewClient(*socket)
	fixtures := []struct {
		language, source string
		line             int
		needle, want     string
	}{
		{"go", "package main\nimport \"fmt\"\nfunc main(){fmt.Pr}\n", 2, "fmt.Pr", "Println"},
		{"python", "import os\nos.pa\n", 1, "os.pa", "path"},
		{"csharp", "using System;\nclass P { static void Main(){Console.Wri} }", 1, "Console.Wri", "WriteLine"},
	}
	for _, f := range fixtures {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			panic(err)
		}
		line := strings.Split(f.source, "\n")[f.line]
		req := analyzer.AssistRequest{Owner: 900001, Task: "latency-check", ClientID: hex.EncodeToString(random[:]), Language: f.language, Source: f.source, Revision: 1, Action: "completion", Position: analyzer.Position{Line: f.line, Character: strings.Index(line, f.needle) + len(f.needle)}}
		failed := false
		for attempt := 0; attempt < 3; attempt++ {
			started := time.Now()
			var response analyzer.AssistResponse
			var err error
			deadline := time.Now().Add(150 * time.Second)
			for time.Now().Before(deadline) {
				requestDeadline := time.Now().Add(55 * time.Second)
				if deadline.Before(requestDeadline) {
					requestDeadline = deadline
				}
				ctx, cancel := context.WithDeadline(context.Background(), requestDeadline)
				response, err = client.Assist(ctx, req)
				cancel()
				if err != analyzer.ErrBusy && !response.Pending {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}
			if err == nil && response.Pending {
				err = context.DeadlineExceeded
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", f.language, err)
				failed = true
				break
			}
			var list struct{ Items []struct{ Label string } }
			if json.Unmarshal(response.Result, &list) != nil {
				_ = json.Unmarshal(response.Result, &list.Items)
			}
			found := false
			for _, item := range list.Items {
				if strings.HasPrefix(item.Label, f.want) {
					found = true
					break
				}
			}
			fmt.Printf("%s · запрос %d: %d мс · очередь %d · подготовка %d · анализ %d мс · %s: %t\n", f.language, attempt+1, time.Since(started).Milliseconds(), response.QueueMS, response.StartupMS, response.AnalysisMS, f.want, found)
			if !found {
				failed = true
				break
			}
		}
		req.Action = "close"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _ = client.Assist(ctx, req)
		cancel()
		if failed {
			os.Exit(1)
		}
	}
	fmt.Println("Подсказки всех трёх языков проверены.")
}

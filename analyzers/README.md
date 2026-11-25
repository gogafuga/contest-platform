# Language assistance (local development)

The editor supports Go, Python and C# through a separate `analyzerd` supervisor.
The public Go API uses the existing participant token, checks task access, then
calls a private Unix socket. Browser-to-API communication is HTTP; the supervisor
uses standard LSP over stdin/stdout. No arbitrary LSP methods, executable commands,
project files, environment variables, image names or filesystem paths are accepted
from a participant. Only one source file per editor session is supported.

## Start

From the repository root:

```bash
./scripts/analyzers.sh build
./scripts/analyzers.sh start
```

The build downloads the language-server images and packages. The start command
builds an ordinary Go binary as your user, then runs only the supervisor with
`sudo` to access Docker. Leave that terminal running. Start/restart the normal
backend as your regular user, using the existing database URL, and run Vite normally.
Do not run the public API or frontend with sudo.

The supervisor creates `/run/contest-analyzers/lsp.sock`, writable only by root
and the launching user's primary group. `ANALYZER_SOCKET` can override the backend
path. Do not make the socket world-writable or expose it over the network. A lock
prevents two supervisors from managing the same container set. Ctrl+C shuts down
the supervisor and removes its containers. Startup also removes orphaned containers
with the service's fixed `devsec.analyzer=true` label. Ordinary project containers
and databases are not touched.

Default capacity is two containers, maximum two editor sessions per participant.
Use `./scripts/analyzers.sh start --max-sessions 4` only if the host has sufficient
CPU/RAM. Containers expire after 90 seconds without requests or 10 minutes total.
Completion and diagnostics recreate an expired session when needed. Request limits
are in-process (4 requests/second with burst 12, two in flight per participant).

## Editor behavior

- Type normally or press Ctrl+Space for completions.
- Hover over a symbol for documentation; type `(` for parameter hints.
- Accept Go/Python autoimport completion suggestions to insert their imports.
- For C#, imports such as `using System;` are available through the lightbulb / Ctrl+.
  The pinned csharp-ls release does not show types from unimported namespaces in
  its completion list; do not present this as identical to VS Code's C# extension.
- Shift+Alt+F requests formatting if the language server supports it.
- Diagnostics underline errors. They do not affect grading or execute the program.
- Editor help has a separate status; unavailable analysis does not disable drafts.

## Isolation and limits

Each session has a separate container with no network or host binds, user 65532,
a read-only root filesystem, all capabilities dropped, no-new-privileges, the
runtime's default seccomp profile, and no Docker socket or application secrets.
Limits: 1 CPU, 1 GiB memory/no extra swap, 128 processes, 256 open files, 16 MiB
per file, 384 MiB temporary storage and 32 MiB workspace. Container capacity stays
reserved until cleanup completes. Source is limited to 256 KiB, LSP messages to
1 MiB and individual requests to 50 seconds. Project/module/package configuration
is generated from trusted templates; runtime package downloads are disabled.

Go uses gopls 0.24.0 / Go 1.27; Python uses pylsp 1.15.0 with Rope; C# uses csharp-ls
0.27.0 / .NET 10. Language-server packages are pinned, but base tags and transitive
Python dependencies are not a fully reproducible lock; freeze image digests after
validation on the deployment host.

**This is a local development isolation baseline, not a production sandbox signoff.**
Docker shares the host kernel. Language-server and compiler vulnerabilities remain
possible, and the supervisor has Docker-level host privileges. Before accepting
hostile submissions in a real deployment, put analyzers on a separate runner host
and validate stronger isolation such as gVisor/microVM, resource limits and cleanup
under adversarial load. A runtime can be selected with `--runtime runsc` after it
has actually been installed and verified. There is no unconfined/privileged fallback.
User programs are never intentionally run, but analysis itself processes untrusted
input and .NET loads the fixed SDK project, so it still needs this isolation.

This default setup connects a locally running backend. A backend inside Compose
needs an explicit, permission-correct bind of only the private supervisor socket;
the Docker socket must never be mounted into the app container.

## Checks

```bash
cd backend
go test ./...
go test -race ./internal/analyzer
```

Optional real-server tests use fixed trusted fixtures, not participant input:
`CONTEST_GOPLS=/path/to/gopls`, `CONTEST_PYLSP_LIBS=/path/to/pylsp/libs`, or
`CONTEST_CSHARP_LS=/path/to/csharp-ls CONTEST_DOTNET=/path/to/sdk` with
`go test ./internal/analyzer -run TestLocal -v`.
API integration tests additionally check authorization, ownership and unavailable
supervisor handling using a temporary database and a private fake supervisor.

C# startup prepares the fixed solution and restore assets at image-build time.
Each new sandbox copies these trusted assets into its private /work, avoiding
three dotnet project-preparation commands on every editor connection. Rebuild
contest-csharp-ls:local and restart analyzerd to activate this optimization.

## Measuring editor startup

From backend, run `go run ./cmd/check-assist` while analyzerd is running. It sends
fixed fixtures through the private Unix socket and checks real member completion
for Go Println, Python os.path and C# Console.WriteLine, timing first and repeated
requests. It closes every synthetic session and does not touch participant data.
A busy service may need time to release an earlier session before the next check.

Build-time indexes are read-only image assets copied privately for each session:
Go standard library cache, Python Rope library index and Jedi cache, and C# fixed
solution/restore assets. Participant code/cache changes never become image assets
or cross into another session. Rebuild all analyzer images to activate changes.

The browser caches completion only for the same document revision/position for
2 seconds (maximum 8 entries), clearing it on edits/disposal. It no longer sends
periodic keepalives for idle editors: an open page must not monopolize capacity.
The 90-second ordinary idle and 10-minute hard lifetime remain unchanged.
The supervisor now requires and verifies gVisor runsc by default.

## Sharing limited sessions

Admission to new workspaces uses a bounded FIFO queue (32 waiting workspaces,
at most two waiting per owner). A full slot returns a short successful
pending/queued response after at most one second. Polling the same editor keeps
its FIFO place, without retaining source text or tokens. Tickets expire after
5 seconds without polling; explicit editor close also removes the ticket.
Admission wait is recorded across polls, separately from startup/analysis.
Under pressure, a workspace with no in-flight requests may be reclaimed after
1 second idle after a completed result (3 seconds for pending project diagnostics),
or after a 20-second lease when other editors are waiting.
Running requests and initializing sessions are protected; cleanup still counts
towards MaxSessions, so no replacement container starts before removal finishes.
A cleanup failure disables further admission. Per-owner maximum remains two.

The dev launcher selects up to six containers from measured CPU/RAM headroom;
manual analyzerd still defaults to two. Each container's CPU/memory/filesystem,
runtime and ownership restrictions are unchanged. On the deployment machine
(12 logical CPUs, ~25 GiB available RAM) the planner selects six, allowing at
most 6 GiB of analyzer container memory plus the existing two 1 GiB runners.
No participant workspace is shared across users.

HTTP request admission is bounded separately at 32 + 2 * MaxSessions (44 with
six sessions), rather than the old unrelated ceiling of 16. This lets a class
reach the bounded FIFO without premature HTTP 429 responses. Request body,
per-owner rate/in-flight limits and the 32-workspace waiting bound remain.
GET /status on the private Unix socket reports only counts/limits and admission
state. It exposes no source, owner IDs or credentials. check-load records these
limits and busy-response reasons for diagnosing the actual deployed configuration.
Clients send full source on each request, so a reclaimed workspace can reopen
privately without losing the separately persisted draft. No workspace or
participant cache is reused between owners. Reopening can still incur cold-start
latency: this is fair sharing, not a guarantee of instant hints for a large class.

Canceled requests that have never returned pending are removed; retained tickets
survive canceled later polls until explicit close or expiry. A full queue returns the
existing busy response; resource bounds, mandatory gVisor, network isolation,
read-only root and ten-minute hard expiry are not relaxed.

Empty cold C# completions without diagnostics are returned as pending for up to
15 seconds, until the first nonempty completion or diagnostics establish
readiness. Legitimate empty results are allowed after this bounded window.
Monaco keeps such lists incomplete and schedules a fresh diagnostic/suggestion
refresh. This avoids presenting a still-loading Roslyn project as a final empty
completion list. The load fixture's C# cursor is placed after the full Wri prefix.

Restart analyzerd after updating Go code (image rebuild is not required). To
exercise retained editors through the real supervisors, run from the repository:

```sh
go -C backend run ./cmd/check-load -participants 5 -retain-assist
```

This retains each test editor until all five have received suggestions, rather
than explicitly freeing every session immediately after its two queries. Only
synthetic sessions are closed; the test database is disposable.

Python's pinned Rope plugin uses the prebuilt library database without repeating
global library scans on initialization/configuration/document-open. Local project
indexing remains enabled; library autoimports and project-name indexing are
verified by test_fast_server.py (also run during image build). No new packages
can be installed into the fixed read-only image at runtime. Each session gets its
own writable copy of the prepared database.
Go's prepared build cache is packed at image build and unpacked privately once,
avoiding a runtime recursive source copy and recursive chmod pass.
Assist responses/check tools report queueMs, startupMs and analysisMs separately;
compare these after rebuilding images and restarting the supervisor before
claiming a measured improvement. C# benefits from shorter queue handoff; no
unverified .NET/runtime flags or resource increases are introduced.

## C# ReadyToRun build

The C# image now publishes the same upstream 0.27.0 source with
PublishReadyToRun=true at image build. The archive is pinned by SHA-256 in both
Dockerfile and build-server.sh. The vendored LSP project explicitly references
the existing centrally pinned FSharp.Core version, avoiding an SDK-bundled-version
conflict; package versions are not upgraded. No trimming is enabled. The fixed
student project is also built once before its obj assets are copied privately.

The published analyzer/Roslyn DLLs were checked for ReadyToRun headers. Trusted
local fixture tests verify diagnostics, completion and import fixes with the
existing .NET heap/CPU settings and 128 MiB file limit. Local first-diagnostic
measurements were 5.12–5.16s for IL and 3.48–4.33s for ReadyToRun. These are host
fixture timings, not gVisor performance or resource-isolation verification.
The real sandbox must be retested after rebuilding the image:

```sh
sudo /usr/bin/docker build -t contest-csharp-ls:local ./analyzers/csharp
# Restart analyzerd, e.g. dev.sh stop, then dev.sh start --lan.
go -C backend run ./cmd/check-load -participants 5 -retain-assist
```

The first image build downloads pinned source/NuGet packages and spends extra
time precompiling the server; runtime remains offline. Container limits, mandatory
gVisor and per-owner workspaces are unchanged. For an IL comparison build, use
the build argument CSHARP_READYTORUN=false; it changes compilation only, never
runtime isolation. ReadyToRun is described in Microsoft's deployment documentation:
https://learn.microsoft.com/en-us/dotnet/core/deploying/ready-to-run

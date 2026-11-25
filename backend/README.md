# Backend

Go, Gin, sqlc/pgx and PostgreSQL 16. Run commands here unless noted otherwise.

```sh
go run main.go
go test ./...
```

Configuration loads from `config.toml`, with uppercase environment variables
overriding the corresponding keys: `DATABASE_URL`, `PORT`, `ADMIN_TOKEN`,
`STATIC_DIR`, `SESSION_TTL_HOURS`, `CONTEST_DURATION_SECONDS`.
The default admin token is for direct local development. The contest launcher
supplies a generated `ADMIN_TOKEN`, which overrides that fallback. `STATIC_DIR`
enables UI static serving from that directory; empty means API-only for local
Vite development. Task 01's threshold is a task-level constant, not an environment
setting.

The production Docker image serves UI and API together on internal port 8080,
published on port 80 by default. Relative API URLs need no CORS middleware.
Static-serving tests cover SPA routes, assets, HEAD requests, missing paths and
API-only mode.

## Database migrations

`db/migrations/001_init.sql` contains the final schema: participants with task
selection, hashed-token sessions, best pass/fail scores, and the shared clock.
The pre-contest migration history was squashed; there are no score-conversion
migrations or legacy line-count columns.

Docker applies the schema only when initializing an empty PostgreSQL volume.
Use `../scripts/contest.sh start` to reset the database and launch the whole stack;
see the root deployment runbook. Existing development volumes are not modified
by changing this SQL file. To rebuild only PostgreSQL manually from the repository
root (deletes its current data):

```sh
docker compose down --volumes
docker compose up -d postgres
```

After changing SQL queries or schema, regenerate the DB bindings:

```sh
sqlc generate
```

## Tasks and judge

The task description is the source of truth. Each task package owns its description,
fixed-seed dataset generator, one reference solver, embedded dataset/expected files,
and tests. See [tools/README.md](tools/README.md) for regeneration.

`model.Task` is a struct containing description data, `Dataset`, and a validation
function. `tasks.Registry` constructs and looks up the three tasks. Descriptions
alone are serialized by metadata endpoints; expected answers are never served.

The generation command runs `Generate()` → `Solve(dataset)` → writes both files.
At runtime, downloads serve embedded datasets. `service.Judge` checks access and
completion, calls the task validator, and saves full points or zero. Generators
and reference solvers do not run on requests.

`model.MatchingValues` sorts and deduplicates lines before comparing them.
Order and repeated matching values do not affect the verdict. Missing or unexpected
values fail. CRLF and empty output lines anywhere are accepted; spaces in nonempty
lines are not trimmed. Submitted files are limited to
2 MiB. Completion uses a simple read before validation, with no transaction or
atomic completion mechanism. Completed-task submissions return 409.

## API

Routes below are relative to `/api/v1`, except `/health`.

| Method | Route | Auth | Purpose |
|---|---|---|---|
| GET | `/health` | Public | Database health |
| POST | `/register` | Public | `{name, group}` → `{token, participant}` |
| GET | `/tasks` | Public | Full task descriptions |
| GET | `/leaderboard` | Public | Task descriptions and participant scores |
| GET | `/clock` | Public | Shared clock state |
| GET | `/me` | Participant Bearer | Participant, selectedTaskId, scores |
| POST | `/tasks/:taskId/select` | Participant Bearer | Select task; 409 if another task is unfinished |
| GET | `/tasks/:taskId/input` | Participant Bearer | Download dataset; route keeps its existing name |
| POST | `/tasks/:taskId/submit` | Participant Bearer | Multipart `file` → `{taskId, passed, feedback, earned}` |
| POST | `/admin/clock/start` | Admin Bearer | Start/restart; optional `{durationSeconds}` |

Participant tokens are opaque random values; only hashes are stored. Admin auth
uses the configured static token. Downloads allow the selected task or completed
tasks. Submissions additionally reject completed tasks. The clock duration override
accepts 60–1209600 seconds (up to 14 days). Participant sessions default to 336 hours
(14 days) from registration. The clock currently does not gate downloads/submissions.

## Tests and remaining work

Tests cover handwritten solver cases, public samples, dataset/answer consistency,
set-based validation, judge retries and completion rejection, selection decisions,
registration/token helpers and clock duration validation.

API integration tests exercise the production router, middleware, services and
real PostgreSQL without mocks. From the repository root:

```sh
./backend/tests/integration/run.sh -v
```

The runner needs Docker and Go. It creates a disposable PostgreSQL 16 container;
each test creates its own database and applies every migration in order. No
persistent volume or development database is used. Ordinary `go test ./...`
excludes these tests. See [tests/integration/README.md](tests/integration/README.md)
for lifecycle, coverage and direct invocation.

Production HTTP wiring lives in `internal/server.New`, shared by `main.go` and
the integration suite.

Unit and API integration tests pass against the squashed schema. The Docker
image passed an HTTP smoke test covering UI/assets, registration, selection,
download, multipart submission and leaderboard scoring. The instructor confirmed
phone access over the LAN. Next is the demo contest; timed difficulty assessment,
full internet-disconnected rehearsal and browser E2E automation remain open.
Rate limiting and a logout endpoint are not implemented.

## Draft languages

Migration `004_draft_languages.sql` adds `language` to drafts and history with a
Go default for existing records. Allowed values are `go`, `python`, `csharp`.
PUT draft accepts `language` with source and expected version; an omitted value
uses Go for compatibility. The service rejects unsupported values and records the
language in the same transaction as the source snapshot. A GET for an unsaved
draft returns Go/version 0. No compiler or interpreter is invoked.

For an existing local database, stop the Go API and apply the new migration once
from `backend/` without resetting its volume:

```sh
docker compose exec -T postgres psql -U contest -d contest01 \
  -v ON_ERROR_STOP=1 --single-transaction < db/migrations/004_draft_languages.sql
```

Use `sudo docker` if required locally; restart the API after migrating.

## Language-server assistance

`POST /api/v1/tasks/:taskId/assist` requires participant Bearer authentication and
existing task access. The checked participant/task overwrite any identity supplied
in the JSON. The API accepts bounded source, language, editor revision and a fixed
action set, then communicates only through `ANALYZER_SOCKET` (default
`/run/contest-analyzers/lsp.sock`). It does not launch Docker or execute user code.

From the repository root, `./scripts/analyzers.sh build` prepares images and
`./scripts/analyzers.sh start` runs the separate local supervisor. Start the public
backend as the ordinary user. No database migration is needed for this feature.
The supervisor socket is privileged IPC; see `../analyzers/README.md` for permissions,
resource limits, supported editor features, production gaps and real-server tests.

Code execution: authenticated POST `/api/v1/tasks/:taskId/runs` accepts
`{requestId, language, sourceCode}`, returns 202 with a run ID; GET on
`/api/v1/tasks/:taskId/runs/:runId` polls the owner's result. See
[runner setup and limits](../runners/README.md). Apply migration 006 before use.

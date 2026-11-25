# API integration tests

From the repository root:

```sh
./backend/tests/integration/run.sh -v
```

Requires Docker running and the Go version in `backend/go.mod`. The runner uses
`postgres:16-alpine`; pull that image and download Go modules before running
offline. No additional Go test dependencies are needed.

The runner creates a new PostgreSQL container with temporary in-memory storage,
no persistent volume, and a random port bound to `127.0.0.1`. It waits for readiness,
sets `TEST_DATABASE_URL`, and runs:

```sh
go test -tags=integration -count=1 -timeout=3m ./tests/integration/...
```

Additional arguments are passed to `go test`, for example:

```sh
./backend/tests/integration/run.sh -v -run TestClock
./backend/tests/integration/run.sh -shuffle=on
```

An exit trap removes the container after success, test failure, or a normal
interrupt (Ctrl+C/SIGTERM). As with any cleanup trap, SIGKILL or a host crash can
leave a stopped/running container behind; the script does not create persistent
volumes. Docker's `--rm` removes it when it stops.

Each top-level test runs in parallel with its own randomly named database. The Go
helper applies every file in `backend/db/migrations/` in filename order, starts
`internal/server.New` with `httptest.NewServer`, and drops only its own database
after closing its servers and connection pool. Test order and previous suite runs
do not affect state. Both the executable and tests use the same router wiring.

Tests perform real HTTP requests through middleware, handlers, services, generated
sqlc queries and PostgreSQL. Successful uploads use committed expected outputs;
existing solver unit tests independently check task rules. Downloads are compared
with the committed datasets. The suite does not call reference solvers.

Coverage:

- Registration, normalized duplicate names, missing fields, working/invalid/expired
  participant tokens, and authentication on every participant route.
- Public task metadata, unknown tasks, selection, locked downloads/submissions,
  repeat selection, retries, all three completions and 375 total points.
- Dataset bytes/download filenames, real multipart uploads, reordered/duplicated
  CRLF answers, completed-task rejection and retained download access.
- Missing/oversized files and successful retry afterward.
- Participant isolation, leaderboard totals/per-task scores, ranking and ties.
- Clock authentication, initial/running/ended states, duration bounds, rejected
  updates, restart without a body, and persistence across server recreation.
- Participant selection, session and scores across server recreation.

Time-dependent states are arranged through SQL in each test's database instead
of sleeping for session expiry or contest end. The clock does not currently gate
submissions; these tests do not introduce that requirement.

To use an already provisioned **disposable test PostgreSQL** instead, run from
`backend/`:

```sh
TEST_DATABASE_URL='postgres://user:password@127.0.0.1:5432/postgres?sslmode=disable' \
  go test -tags=integration -count=1 -timeout=3m ./tests/integration/...
```

The account must be able to create and drop databases. The supplied database is
used only for administration; tests create their own databases. Missing
`TEST_DATABASE_URL` fails explicitly. Ordinary `go test ./...` omits this suite
because its Go files have the `integration` build tag.

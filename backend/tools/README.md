# Contest datasets

From `backend/`, regenerate with:

```sh
go run ./tools/generate-datasets
go test ./...
```

The published `description.go` is the source of truth for each task. Generators,
solvers and explicit test expectations must follow it; discuss rule changes before
changing the description to accommodate implementation.

Each task owns `Generate()` in `internal/tasks/task_0N/generate.go` and a reference
`Solve(dataset)` in its task implementation. `Generate()` produces dataset only.
The command serializes that dataset, passes it to `Solve`, and writes `data/dataset.txt`
and `data/expected.txt`. There is one implementation of the solving rules per task.

Generators use fixed seeds (20261108 plus task number) and shared task constants.
Review and commit both files together. The generator implementation is part of
reproducibility; committed files are the runtime source of truth.

Each task embeds its files with `go:embed`. Requests serve the existing dataset;
validation compares submitted output with the existing answer. No generation or
reference solving runs on requests. Only description metadata is serialized by
the API; answers are not exposed through HTTP. The source repository contains the
answers and should not be distributed to participants before the contest.

| Task | Dataset | Coverage |
|---|---|---|
| 01 | 9,548 lines, 600 distinct IPs; 450 output IPs | Bursts of 4/5/6/12, counts across different seconds, shuffled events, invalid/blank records, unique output |
| 02 | 5,009 passwords; 4,005 unique passwords violating at least one requirement | Lengths 7/8/64/65, each missing character class, spaces/tabs, duplicates |
| 03 | 5,010 IPv4 addresses; 2,703 unique addresses outside `172.16.32.0/20` | Subnet endpoints, nearby addresses on both sides, addresses across IPv4, duplicates |

Task 01's threshold is fixed at five events per IP per calendar second, matching
its statement and sample. To change it, edit the task-level `threshold` constant in `task_01.go` and
regenerate. The description, sample, generator, and dataset tests use that constant. Dataset sizes are an initial choice;
a timed student-level walkthrough should still confirm difficulty for 90 minutes.

Tests cover:

- Handwritten inputs with explicit expected results verify solver correctness.
- The fixed-seed generator reproduces the stored dataset.
- The solver reproduces the stored answer (a consistency check, not an independent solver).
- Public examples match the solver.
- Stored answers pass validation in any order, extra output fails, and sample answers cannot solve the dataset.

During the contest, the backend serves embedded dataset and validates submissions
against embedded expected output. Generators and reference solvers do not run.

All tasks ask for unique matching values in any order. The server sorts and
deduplicates submitted and expected lines before comparison. Repeated matching
values do not affect the verdict; missing or unexpected values fail.

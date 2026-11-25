# Contest frontend

React + TypeScript + Vite, CSS Modules, bundled JetBrains Mono and IBM Plex Sans.
Task metadata, sessions, selection, judging, leaderboard and clock use the backend.

```sh
npm install
npm run dev
npm run build
npm run lint
```

API requests use relative `/api/v1` URLs by default. The Docker image serves this
bundle from Go on the same port as the API (public port 80 by default); no LAN
address is embedded in JS, and no CORS configuration is needed.
`VITE_API_URL` is an optional build-time override, normally left empty.

For local development, Vite proxies `/api` and `/health` to `localhost:8080`.
Run the Go backend on that port, then `npm run dev`. LAN browsers can connect to
`npm run dev -- --host 0.0.0.0` without configuring a separate API address.
Fonts and all runtime assets are bundled locally. Production SPA fallback is
handled by Go; see the root deployment runbook for the Docker launcher.

## Behavior

- `/register`: name and group registration.
- `/tasks`: task tabs with a two-column statement/editor workspace. One selected task must be completed before selecting
  another; the server enforces the rule. Solved tasks remain readable/downloadable.
- `/leaderboard`: polls the shared leaderboard every four seconds.
- `/clock`: admin-token-gated start/restart controls. The header clock is read-only.

Cards show the task's full point value (125 / 100 / 150), with `решено` or `не решено`.
Solved cards keep a green border on hover; solved carousel dots are green. The active
dot is larger, with a size transition and immediate color changes to avoid flashing.
The task-number label stays amber. Cards scroll internally without changing height.

Submissions contain only the participant's output file. Results are pass/fail with
feedback and full points or zero. Failed submissions can be retried; solved-task
upload and submit controls are disabled. Dataset downloads stay available after
completion. All tasks ask for unique matching values in any order.

`localStorage` caches participant/token, scores and selection. `GET /me` restores
server-authoritative scores/selection; 401 clears the session. Task descriptions
come from `GET /tasks`, never a duplicated frontend rules file. Admin token and
chosen clock duration are cached for the instructor's convenience.

The waiting/ended clock polls about every 1.5–2.5 seconds; while running it counts
down locally and resyncs about every 30–40 seconds. The clock is informational:
the backend currently does not reject submissions based on contest time.
The admin can choose up to 20,160 minutes (14 days). The countdown uses
`DD:HH:MM:SS`, omitting leading zero days and hours but always retaining `MM:SS`.
For example: `07:00:00:00`, `23:59:59`, `59:59`, `00:59`, then `00:00` at expiry.

## Verification

`npm run build` checks TypeScript and builds the bundle. `npm run lint` currently
reports a history-loading effect warning plus two existing warnings: state set in the carousel effect, and the context
module exporting a hook alongside its provider. `npm test` (Node.js 22.6+ with
TypeScript stripping) checks countdown-format boundaries. Local headless-browser checks were used for task-selection
click handling and dot transitions. The instructor confirmed the bundled site
works from their phone over the LAN, and the contest has since run successfully.
A full internet-disconnected browser rehearsal and browser E2E automation remain
open.

## Draft editor and history

The task card includes a locally bundled Monaco editor with Go, Python and C#
syntax highlighting. Language and source share the draft version and are saved
through GET/PUT `/api/v1/tasks/:taskId/draft`. Pending changes have a local backup;
a 409 stops saving until the participant resolves the conflict. Switching language
keeps the current source text. One draft is stored per participant/task, not per language.
Downloading source uses `.go`, `.py` or `.cs`. Code execution is not implemented.

`/history` is the instructor viewer (also linked from `/clock`). Enter an admin
token, search/select a participant by name and group, and choose a task; the page lists the latest 100 snapshots, offers a
version slider, and compares two snapshots with Monaco's read-only diff viewer.
Older-history pagination is not yet implemented. History
tokens are kept in page memory and admin authorization is enforced by the API.
The language picker requires migration `004_draft_languages.sql` and a restarted
backend. Existing drafts/history default to Go.

## Participant event monitoring

On `/tasks`, a notice is shown before collecting events. After acknowledgement,
copy/paste and document visibility changes are associated with the selected task.
Clipboard text is never read. The queue sends events sequentially, preserves UUIDs
on retries, backs off on failures, and respects `Retry-After` on 429 responses.
The browser queue is limited to 500 records per tab/session; restored records older
than 24 hours are discarded. Closing a tab before delivery can still lose events;
client timestamps and browser reports are not trusted proof of participant actions.

The `/history` instructor view includes event counts for the loaded records and a
paginated event table (100 per request), with browser/server timestamps and relation
to the selected code snapshot. Counts are not contest-wide totals. Click the events
refresh button to retrieve newer records; the list is not polled automatically.

Server-side retention/deletion and a final organizer retention policy remain to be
implemented before using this monitoring for a real contest. Existing backend event
rate limits are in-process, not shared between multiple backend instances.

Participant search uses the admin-only GET `/api/v1/admin/participants?q=...`,
returning up to 20 participants. Matches are literal case-insensitive substrings
of the registered name. The picker debounces requests by 300 ms, aborts superseded
searches, and supports arrow keys/Enter/Escape. A student must be explicitly selected
before loading history. The API route requires a backend restart, no new migration.

## Language assistance

Monaco uses the authenticated task `POST /api/v1/tasks/:taskId/assist` endpoint for
Go/Python/C# completion, diagnostics, hover, parameter hints, formatting and text-only
code actions. Analysis is debounced independently of draft saving. Source revisions
prevent late responses from replacing newer diagnostics. Server commands, links
marked trusted, and edits to other documents are never executed/applied by the bridge.
The supervisor must be running; otherwise the editor shows a separate help status
and draft saving continues. See `../analyzers/README.md` for startup and limitations.

The task workspace now uses the full available width: statement and submission
controls scroll on the left, Monaco occupies the right pane and remaining height.
Below 900 px the panes stack vertically. Task switching keeps mounted drafts to
avoid discarding edits. Monaco 0.57 requires the explicit suggestController/core
command imports in addition to feature registration; these are loaded locally.
The live browser check verified ReadFile suggestions and automatic import os.

The editor's run button saves a draft snapshot and submits an idempotent execution
request. Results are polled and restored after refresh; successful judging updates
the participant's score. Input is available as input.txt, task-ID.txt and stdin;
programs print the answer to stdout. The private runner must be configured as
explained in [runners/README.md](../runners/README.md).

The current task workspace exposes statement/output tabs on the left and language
starter templates in empty drafts. The manual answer upload UI is removed.
Run `./scripts/prepare-editor.sh` from the repository root to rebuild the changed
Go runner cache/C# analyzer images and start analyzers under gVisor in foreground.
The regular backend, frontend and runner remain separate services. The Go cache
speed improvement must be measured after image rebuild; UI total time includes compilation.

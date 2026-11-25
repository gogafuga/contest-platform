-- name: InitContestState :exec
INSERT INTO contest_state (id, duration_seconds)
VALUES (1, $1)
ON CONFLICT (id) DO NOTHING;

-- name: GetContestState :one
SELECT * FROM contest_state WHERE id = 1;

-- name: StartContest :one
UPDATE contest_state
SET started_at = NOW(),
    duration_seconds = COALESCE(sqlc.narg('duration_seconds'), duration_seconds),
    updated_at = NOW()
WHERE id = 1
RETURNING *;

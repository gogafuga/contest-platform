-- name: GetDraft :one
SELECT * FROM drafts
WHERE participant_id = $1 AND task_id = $2;

-- name: CreateDraft :one
INSERT INTO drafts (participant_id, task_id, source_code, language)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateDraft :one
UPDATE drafts
SET source_code = $3,
    language = $5, 
    version= version + 1,
    updated_at = NOW()
WHERE participant_id = $1 AND task_id = $2 AND version = $4
RETURNING *;

-- name: CreateParticipantEvent :exec
INSERT INTO participant_events (
    participant_id,
    task_id,
    event_id,
    event_type,
    client_at
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (participant_id, event_id) DO NOTHING;

-- name: ListParticipantEvents :many
SELECT *
FROM participant_events
WHERE participant_id = $1
  AND task_id = $2
  AND id > $3
ORDER BY id
LIMIT $4;
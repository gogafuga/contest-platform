-- name: CreateDraftHistory :one
INSERT INTO draft_history(participant_id, task_id, source_code, version, language)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListDraftHistory :many
SELECT version, created_at, language FROM draft_history
WHERE participant_id = $1 AND task_id = $2
ORDER BY version DESC
LIMIT $3;

-- name: GetDraftHistoryVersion :one
SELECT * FROM draft_history
WHERE participant_id = $1 AND task_id = $2 AND version = $3;

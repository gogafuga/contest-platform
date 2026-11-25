-- name: CreateCodeRun :one
INSERT INTO code_runs (participant_id, request_id, task_id, language, source_code)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (participant_id, request_id) DO NOTHING
RETURNING *;

-- name: GetCodeRunByRequest :one
SELECT * FROM code_runs WHERE participant_id = $1 AND request_id = $2;

-- name: GetCodeRun :one
SELECT * FROM code_runs WHERE id = $1 AND participant_id = $2;

-- name: StartCodeRun :exec
UPDATE code_runs SET status = 'running', started_at = NOW() WHERE id = $1 AND status = 'queued';

-- name: FinishCodeRun :one
UPDATE code_runs SET status = $2, stdout = $3, stderr = $4, exit_code = $5,
    duration_ms = $6, passed = $7, earned = $8, feedback = $9, output_truncated = $10, finished_at = NOW()
WHERE id = $1 AND status IN ('queued', 'running') RETURNING *;

-- name: InterruptCodeRun :one
UPDATE code_runs SET status = 'interrupted', finished_at = NOW(), feedback = 'Сервер был перезапущен. Запустите решение снова.'
WHERE id = $1 AND status IN ('queued', 'running') RETURNING *;

-- name: CodeRunCounts :one
SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE participant_id = $1) AS personal
FROM code_runs;

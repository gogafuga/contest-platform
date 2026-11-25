-- name: GetTaskTiming :one
SELECT t.started_at, t.estimated,
    (CASE WHEN s.score >= sqlc.arg(points)::double precision THEN s.submitted_at END)::timestamptz AS finished_at,
    NOW()::timestamptz AS observed_at
FROM participant_task_timing t
LEFT JOIN submissions s USING (participant_id, task_id)
WHERE t.participant_id = $1 AND t.task_id = $2;

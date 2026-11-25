-- name: UpsertBestSubmission :one
INSERT INTO submissions (participant_id, task_id, score)
VALUES ($1, $2, $3)
ON CONFLICT (participant_id, task_id) DO UPDATE SET
 score = GREATEST(submissions.score, EXCLUDED.score),
 submitted_at = CASE WHEN EXCLUDED.score > submissions.score THEN NOW() ELSE submissions.submitted_at END
RETURNING *;

-- name: GetSubmissionScore :one
SELECT score FROM submissions WHERE participant_id = $1 AND task_id = $2;

-- name: ListLeaderboard :many
SELECT
    p.id,
    p.name,
    p.group_name,
    COALESCE(jsonb_object_agg(s.task_id, s.score) FILTER (WHERE s.task_id IS NOT NULL), '{}'::jsonb) AS scores,
    COALESCE(SUM(s.score), 0)::double precision AS total
FROM participants p
LEFT JOIN submissions s ON s.participant_id = p.id
GROUP BY p.id, p.name, p.group_name
ORDER BY total DESC, p.name ASC;

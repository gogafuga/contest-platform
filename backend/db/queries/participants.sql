-- name: CreateParticipant :one
INSERT INTO participants (name, group_name)
VALUES ($1, $2)
RETURNING *;

-- name: GetParticipantByID :one
SELECT * FROM participants WHERE id = $1;

-- name: SetSelectedTask :one
WITH selected AS (
    UPDATE participants SET selected_task_id = $2
    WHERE id = $1
    RETURNING *
), timing AS (
    INSERT INTO participant_task_timing (participant_id, task_id)
    SELECT id, selected_task_id FROM selected
    ON CONFLICT (participant_id, task_id) DO NOTHING
)
SELECT * FROM selected;

-- name: GetParticipantState :one
SELECT
    p.id,
    p.selected_task_id,
    COALESCE(jsonb_object_agg(s.task_id, s.score) FILTER (WHERE s.task_id IS NOT NULL), '{}'::jsonb) AS scores
FROM participants p
LEFT JOIN submissions s ON s.participant_id = p.id
WHERE p.id = $1
GROUP BY p.id, p.selected_task_id;

-- name: SearchParticipants :many
SELECT id, name, group_name
FROM participants
WHERE strpos(lower(name), lower(sqlc.arg(search)::text)) > 0
ORDER BY name_norm, id
LIMIT 20;

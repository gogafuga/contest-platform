-- name: CreateSession :one
INSERT INTO sessions (participant_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT
    s.id AS session_id,
    s.participant_id,
    s.token_hash,
    s.expires_at,
    p.name,
    p.group_name
FROM sessions s
JOIN participants p ON p.id = s.participant_id
WHERE s.token_hash = $1 AND s.expires_at > NOW()
LIMIT 1;

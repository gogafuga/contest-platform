CREATE TABLE IF NOT EXISTS participant_task_timing (
    participant_id BIGINT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    estimated BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (participant_id, task_id)
);

-- Old selections were not timestamped. Preserve the earliest available server
-- evidence, explicitly marked as an estimate rather than an exact selection.
INSERT INTO participant_task_timing (participant_id, task_id, started_at, estimated)
SELECT participant_id, task_id, MIN(observed_at), TRUE
FROM (
    SELECT participant_id, task_id, created_at AS observed_at FROM draft_history
    UNION ALL
    SELECT participant_id, task_id, received_at FROM participant_events
    UNION ALL
    SELECT participant_id, task_id, created_at FROM code_runs
    UNION ALL
    SELECT participant_id, task_id, submitted_at FROM submissions
) activity
GROUP BY participant_id, task_id
ON CONFLICT (participant_id, task_id) DO NOTHING;

CREATE TABLE draft_history (
    id             BIGSERIAL        PRIMARY KEY,
    participant_id BIGINT           NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    task_id        TEXT             NOT NULL,
    source_code TEXT        NOT NULL DEFAULT '',
    version     BIGINT      NOT NULL CHECK (version > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (participant_id, task_id,version)
);

INSERT INTO draft_history (
    participant_id,
    task_id,
    source_code,
    version,
    created_at
)
SELECT
    participant_id,
    task_id,
    source_code,
    version,
    updated_at
FROM drafts;
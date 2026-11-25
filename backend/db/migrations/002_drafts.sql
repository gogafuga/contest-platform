CREATE TABLE drafts (
    id             BIGSERIAL        PRIMARY KEY,
    participant_id BIGINT           NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    task_id        TEXT             NOT NULL,
    source_code TEXT        NOT NULL DEFAULT '',
    version     BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (participant_id, task_id)
);
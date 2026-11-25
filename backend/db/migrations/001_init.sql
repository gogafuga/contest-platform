CREATE TABLE participants (
    id          BIGSERIAL   PRIMARY KEY,
    name        TEXT        NOT NULL,
    name_norm   TEXT        GENERATED ALWAYS AS (lower(btrim(name))) STORED,
    group_name  TEXT        NOT NULL,
    selected_task_id TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name_norm)
);

CREATE TABLE sessions (
    id             BIGSERIAL   PRIMARY KEY,
    participant_id BIGINT      NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    token_hash     TEXT        NOT NULL UNIQUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_token_hash_idx     ON sessions(token_hash);
CREATE INDEX sessions_participant_id_idx ON sessions(participant_id);

CREATE TABLE submissions (
    id             BIGSERIAL        PRIMARY KEY,
    participant_id BIGINT           NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    task_id        TEXT             NOT NULL,
    score          DOUBLE PRECISION NOT NULL,
    submitted_at   TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    UNIQUE (participant_id, task_id)
);
CREATE INDEX submissions_participant_id_idx ON submissions(participant_id);

CREATE TABLE contest_state (
    id               SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    started_at       TIMESTAMPTZ,
    duration_seconds INTEGER     NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

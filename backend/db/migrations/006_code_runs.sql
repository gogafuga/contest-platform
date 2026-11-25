CREATE TABLE code_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    participant_id BIGINT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    request_id UUID NOT NULL,
    task_id TEXT NOT NULL,
    language TEXT NOT NULL CHECK (language IN ('go', 'python', 'csharp')),
    source_code TEXT NOT NULL CHECK (octet_length(source_code) BETWEEN 1 AND 262144),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','accepted','wrong_answer','compile_error','compile_timeout','runtime_error','time_limit','memory_limit','output_limit','system_error','interrupted')),
    stdout BYTEA NOT NULL DEFAULT ''::bytea,
    stderr BYTEA NOT NULL DEFAULT ''::bytea,
    output_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    exit_code INTEGER NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    passed BOOLEAN NOT NULL DEFAULT FALSE,
    earned DOUBLE PRECISION NOT NULL DEFAULT 0,
    feedback TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE (participant_id, request_id)
);
CREATE INDEX code_runs_participant_created_idx ON code_runs (participant_id, created_at DESC);

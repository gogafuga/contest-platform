CREATE TABLE participant_events (
    id             BIGSERIAL   PRIMARY KEY,
    participant_id BIGINT      NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    task_id        TEXT        NOT NULL,
    event_id       UUID        NOT NULL,
    event_type     TEXT        NOT NULL,
    client_at      TIMESTAMPTZ NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (participant_id, event_id),
    CHECK (event_type IN ('copy', 'paste', 'tab_hidden', 'tab_visible'))
);

CREATE INDEX participant_events_timeline_idx
    ON participant_events (participant_id, task_id, received_at, id);
ALTER TABLE drafts
    ADD COLUMN language TEXT NOT NULL DEFAULT 'go'
    CHECK (language IN ('go', 'python', 'csharp'));

ALTER TABLE draft_history
    ADD COLUMN language TEXT NOT NULL DEFAULT 'go'
    CHECK (language IN ('go', 'python', 'csharp'));

ALTER TABLE tasks ADD COLUMN locked_by TEXT;
ALTER TABLE tasks ADD COLUMN locked_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN max_retries INTEGER NOT NULL DEFAULT 3;
ALTER TABLE tasks ADD COLUMN last_error TEXT;
ALTER TABLE tasks ADD COLUMN next_retry_at TIMESTAMPTZ;

CREATE INDEX idx_tasks_dispatchable ON tasks (status, next_retry_at)
WHERE status = 'pending' AND locked_by IS NULL;
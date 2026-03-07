DROP INDEX idx_tasks_dispatchable;

ALTER TABLE tasks DROP COLUMN next_retry_at;
ALTER TABLE tasks DROP COLUMN last_error;
ALTER TABLE tasks DROP COLUMN max_retries;
ALTER TABLE tasks DROP COLUMN attempt_count;
ALTER TABLE tasks DROP COLUMN locked_at;
ALTER TABLE tasks DROP COLUMN locked_by;
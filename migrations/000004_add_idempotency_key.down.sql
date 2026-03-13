DROP INDEX IF EXISTS idx_tasks_idempotency_key;

ALTER TABLE tasks DROP COLUMN idempotency_key;
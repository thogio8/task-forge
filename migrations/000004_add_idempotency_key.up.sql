ALTER TABLE tasks ADD COLUMN idempotency_key VARCHAR(255);

CREATE UNIQUE INDEX idx_tasks_idempotency_key
ON tasks(idempotency_key)
WHERE idempotency_key IS NOT NULL;
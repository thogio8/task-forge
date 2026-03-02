DROP TRIGGER trigger_update ON tasks;

DROP FUNCTION update_updated_at_column;

DROP INDEX idx_created_at;

DROP INDEX idx_status;

ALTER TABLE tasks DROP CONSTRAINT check_status_values;

ALTER TABLE tasks
ALTER COLUMN payload DROP DEFAULT;

ALTER TABLE tasks
ALTER COLUMN payload TYPE TEXT USING payload::TEXT;

ALTER TABLE tasks
ALTER COLUMN payload SET DEFAULT '';
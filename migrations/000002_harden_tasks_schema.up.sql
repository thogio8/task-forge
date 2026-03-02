UPDATE tasks
SET payload = '{}'
WHERE payload = '' OR payload IS NULL;

ALTER TABLE tasks
ALTER COLUMN payload DROP DEFAULT;

ALTER TABLE tasks
ALTER COLUMN payload TYPE jsonb USING payload::jsonb;

ALTER TABLE tasks
ALTER COLUMN payload SET DEFAULT '{}'::jsonb;

ALTER TABLE tasks
ADD CONSTRAINT check_status_values 
CHECK (status IN ('pending', 'running', 'completed', 'failed'));

CREATE INDEX idx_status ON tasks (status); 

CREATE INDEX idx_created_at ON tasks (created_at);

CREATE OR REPLACE FUNCTION update_updated_at_column()                                                                                                                                            
RETURNS TRIGGER AS $$                                                                                                                                                                       
BEGIN   
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update
BEFORE UPDATE ON tasks
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

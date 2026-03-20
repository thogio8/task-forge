CREATE TABLE IF NOT EXISTS dead_letter_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    original_task_id UUID NOT NULL,
    payload jsonb NOT NULL,
    last_error TEXT NOT NULL,
    attempt_count INT NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retried_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

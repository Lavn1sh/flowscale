CREATE TABLE scheduled_workflows (
    id VARCHAR(255) PRIMARY KEY,
    workflow_id VARCHAR(255) NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    schedule_type VARCHAR(50) NOT NULL,
    run_at TIMESTAMP WITH TIME ZONE,
    cron_expression VARCHAR(100),
    interval VARCHAR(50),
    next_run_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_scheduled_workflows_next_run_at ON scheduled_workflows(next_run_at) WHERE status = 'ACTIVE';

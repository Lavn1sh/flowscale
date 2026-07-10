CREATE TABLE workflows (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE activities (
    id VARCHAR(255) PRIMARY KEY,
    workflow_id VARCHAR(255) NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    compensation_activity_name VARCHAR(255),
    retry_max_attempts INT,
    retry_backoff_strategy VARCHAR(50),
    timeout VARCHAR(50),
    position INT NOT NULL,
    UNIQUE (workflow_id, name)
);

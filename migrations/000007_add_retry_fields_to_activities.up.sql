ALTER TABLE activities ADD COLUMN retry_initial_interval VARCHAR(50);
ALTER TABLE activities ADD COLUMN retry_backoff_coefficient FLOAT;

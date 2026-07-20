ALTER TABLE activities DROP COLUMN IF EXISTS retry_initial_interval;
ALTER TABLE activities DROP COLUMN IF EXISTS retry_backoff_coefficient;

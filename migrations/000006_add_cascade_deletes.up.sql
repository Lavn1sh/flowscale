ALTER TABLE workflow_executions DROP CONSTRAINT IF EXISTS workflow_executions_workflow_id_fkey;
ALTER TABLE workflow_executions ADD CONSTRAINT workflow_executions_workflow_id_fkey FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE;

ALTER TABLE activity_executions DROP CONSTRAINT IF EXISTS activity_executions_execution_id_fkey;
ALTER TABLE activity_executions ADD CONSTRAINT activity_executions_execution_id_fkey FOREIGN KEY (execution_id) REFERENCES workflow_executions(id) ON DELETE CASCADE;

ALTER TABLE scheduled_workflows DROP CONSTRAINT IF EXISTS scheduled_workflows_workflow_id_fkey;
ALTER TABLE scheduled_workflows ADD CONSTRAINT scheduled_workflows_workflow_id_fkey FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE;

ALTER TABLE workflow_events DROP CONSTRAINT IF EXISTS workflow_events_execution_id_fkey;
ALTER TABLE workflow_events ADD CONSTRAINT workflow_events_execution_id_fkey FOREIGN KEY (execution_id) REFERENCES workflow_executions(id) ON DELETE CASCADE;

# How It Works (Internals)

FlowScale is fundamentally a state machine manager.

## The State Machine

Every Workflow has a defined list of **Activities** (steps). 
An Execution can be in one of the following states:
- `PENDING`
- `RUNNING`
- `COMPLETED`
- `FAILED`
- `COMPENSATED`

Similarly, each Activity inside the execution has a state. The Engine drives the execution forward by tracking which activities have completed.

### Idempotency
Workers might crash mid-execution. Because RabbitMQ guarantees at-least-once delivery, another worker might receive the same task. Workers deduplicate tasks by checking the database (via PostgreSQL transaction) before starting an activity.

## Retries & Dead Letter Queue (DLQ)

If a worker encounters an error during an activity:
1. The Worker checks the configured **Retry Policy** for that activity.
2. If `attempts < MaxAttempts`, the Worker logs the error and allows RabbitMQ to re-queue the message after an exponential backoff.
3. If `attempts >= MaxAttempts`, the Worker signals a `FATAL` error to the Engine.
4. The Engine marks the Activity Execution as `FAILED` and inserts it into the **Dead Letter Queue (DLQ)** table in PostgreSQL.
5. The overall Workflow Execution is paused, marked as `FAILED`.

An operator can manually inspect the DLQ in the Web UI, fix the underlying issue (e.g., bringing a 3rd party API back online), and click **Retry**. The Engine will re-queue the exact activity, resuming the workflow from where it left off!

## Saga Compensations

Distributed systems cannot rely on traditional SQL `ROLLBACK` commands because tasks might span multiple disparate systems (e.g., Stripe, SendGrid, Internal DB). 

FlowScale implements **Saga Compensations**:
- You can define a `CompensationActivity` for any step in your workflow.
- If a subsequent step in the workflow fails terminally, the Engine will automatically walk *backwards* through all completed steps and execute their defined compensation activities.
- Example: If Step 2 (Charge Credit Card) succeeds, but Step 3 (Provision Server) fails, the Engine will execute the Compensation for Step 2 (Refund Credit Card) to ensure the system is left in a consistent state.
- Once compensated, the workflow execution state becomes `COMPENSATED`.

## Scheduling

The built-in Scheduler is a continuous background loop that uses cron expressions. Every minute, it checks the database for any active schedules that should have fired. If a match is found, it injects a "Start Workflow" command into the Engine, ensuring you can run nightly data syncs, hourly backups, etc.

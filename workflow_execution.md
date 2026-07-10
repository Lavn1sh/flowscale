# workflow-execution.md

# Workflow Execution Semantics

## Overview

This document defines the execution semantics of the workflow engine.

It specifies:

* Workflow lifecycle
* Activity lifecycle
* State transitions
* Retry behavior
* Compensation behavior
* Failure handling
* Scheduling behavior
* Idempotency guarantees

These semantics are the source of truth for the orchestration engine.
Where architecture.md's summary diagrams appear to differ, this document
wins.

---

# Core Principles

## Durable Execution

All workflow state changes must be persisted before advancing execution.

No workflow progress may rely solely on memory.

---

## At-Least-Once Activity Delivery

Activities are delivered with at-least-once semantics.

Consequences:

* Activities may execute more than once.
* Activities must be idempotent.

---

## Single Workflow Ownership

At any moment, only one engine instance may modify a workflow execution state.

Distributed locking must enforce this rule.

---

## Event-Driven Progression

Workflow progression is driven by activity completion events.

Activities do not directly modify workflow state.

Workers only report execution results, and they do so by publishing to
the results queue (`activity.results.queue`), not via a direct RPC back
to the engine. The Workflow Engine owns all state transitions.

---

# Workflow Lifecycle

## States

```text
PENDING
RUNNING
COMPLETED
FAILED
COMPENSATING
COMPENSATED
CANCELLED
```

---

## PENDING

Workflow execution record exists.

No activities have started.

Possible transitions:

```text
PENDING -> RUNNING
PENDING -> CANCELLED
```

---

## RUNNING

Workflow is actively executing activities.

Possible transitions:

```text
RUNNING -> COMPLETED
RUNNING -> FAILED
RUNNING -> COMPENSATING
RUNNING -> CANCELLED
```

---

## COMPLETED

Workflow finished successfully.

Terminal state.

---

## FAILED

Workflow failed without compensation.

Terminal state.

Used when:

* No compensation defined
* Compensation disabled
* Compensation was attempted but itself exhausted its retries (manual
  intervention required — see Compensation Failure below)

---

## COMPENSATING

Compensation activities are currently executing.

Possible transitions:

```text
COMPENSATING -> COMPENSATED
COMPENSATING -> FAILED
```

---

## COMPENSATED

All compensations completed successfully.

Terminal state.

---

## CANCELLED

Workflow execution cancelled by user.

Terminal state.

---

# Activity Lifecycle

## States

```text
PENDING
RUNNING
COMPLETED
FAILED
RETRYING
```

Dead-lettering is **not** a sixth state. An activity whose retries are
exhausted stays in `FAILED`; a `dead_lettered_at` timestamp is set on the
`activity_executions` row and the task is routed to the DLQ. Treating
`DLQ` as if it were a transition target (`FAILED -> DLQ`) would conflict
with this enumerated lifecycle, so it is defined here as a queue
destination plus a timestamp, not a status value.

---

## PENDING

Activity has been scheduled but not started.

---

## RUNNING

Worker has received activity.

Execution in progress.

---

## COMPLETED

Activity completed successfully.

Terminal state.

---

## FAILED

Activity execution failed.

Retry decision pending. If retries are exhausted, the row remains
`FAILED` with `dead_lettered_at` set (see above) rather than moving to a
separate state.

---

## RETRYING

Activity scheduled for future retry.

Returns to:

```text
RETRYING -> PENDING
```

---

# Workflow Execution Flow

## Step 1

User starts workflow.

Request:

```text
StartWorkflow(workflow_definition_id)
```

Engine actions:

* Create execution record
* Persist initial state
* Create workflow event
* Transition to RUNNING

---

## Step 2

Schedule first activity.

Engine actions:

* Create activity execution record
* Publish activity task

Activity enters:

```text
PENDING
```

---

## Step 3

Worker receives activity.

Activity enters:

```text
RUNNING
```

Worker executes business logic, sending periodic heartbeats for the
duration of execution (consumed by the timeout-detection reaper — see
Timeout Semantics).

---

## Step 4

Worker reports result.

Possible outcomes:

```text
SUCCESS
FAILURE
TIMEOUT
```

The result is published to the results queue, not delivered via a
synchronous RPC. The original activity task is **not acknowledged** on
the source queue until the Workflow Engine has durably persisted the
result. This avoids a double-execution path: if a synchronous RPC were
used instead and it failed after the activity's side effect had already
succeeded, RabbitMQ would redeliver the unacked task and re-run an
already-completed activity.

---

# Successful Activity

Worker returns success.

Engine actions:

* Persist result
* Mark activity COMPLETED
* Create workflow event

If next activity exists:

```text
Schedule next activity
```

Otherwise:

```text
Workflow -> COMPLETED
```

---

# Failed Activity

Worker returns failure.

Engine actions:

* Persist failure
* Increment retry counter

Retry policy evaluated.

---

# Retry Semantics

## Retry Policy

Example:

```json
{
  "max_attempts": 5,
  "backoff": "exponential"
}
```

---

## Retry Decision

`max_attempts` counts **total attempts**, not retries in addition to an
initial attempt. With `max_attempts: 5`:

* Attempt 1 runs immediately (no delay).
* Attempts 2 through 5 each follow the backoff table below.
* If attempt 5 fails, there is no attempt 6 — the activity is
  dead-lettered.

If:

```text
attempts < max_attempts
```

Then:

```text
FAILED -> RETRYING
```

Otherwise (`attempts >= max_attempts`):

```text
FAILED (dead_lettered_at set, routed to DLQ)
```

The activity execution row's `status` remains `FAILED`; see Dead Letter
Queue Semantics below.

---

## Backoff Strategy

Exponential, applied before attempts 2–5 (attempt 1 has no delay):

```text
before attempt 2 = 1 second
before attempt 3 = 2 seconds
before attempt 4 = 4 seconds
before attempt 5 = 8 seconds
```

Configurable. (Note: this is the delay *before* each attempt, indexed by
the attempt about to run — not a fixed table of 5 delays following 5
attempts, which would imply 6 total attempts. See Retry Decision above
for how this avoids the off-by-one.)

---

## Retry Scheduling

Retry task published after delay, to the appropriate fixed-TTL parking
queue for that backoff tier (see architecture.md RabbitMQ Topology — the
delay queues are per-tier, not a single ordered queue, since RabbitMQ
expires messages in FIFO order and a short-delay message queued behind a
long-delay one would otherwise be stuck waiting).

Activity transitions:

```text
FAILED
   ↓
RETRYING
   ↓
PENDING
```

---

# Timeout Semantics

Activities may define execution timeout.

Example:

```json
{
  "timeout": "30s"
}
```

---

## Timeout Handling

If timeout exceeded:

Activity treated as failed.

Workflow Engine:

* Marks activity failed
* Evaluates retry policy

Timeout participates in retries.

## Timeout Detection Mechanism

A worker that crashes mid-activity cannot self-report a timeout, so
detection cannot rely solely on the worker. A background reaper/sweeper
(part of the Workflow Engine) periodically scans `activity_executions`
for rows in `RUNNING` status whose `started_at + timeout` has elapsed. It
cross-references worker heartbeats: an activity is only treated as timed
out once both the declared timeout has passed *and* no heartbeat has been
received within the expected heartbeat interval. Once identified, the
reaper drives the same Failed Activity path described above (increment
retry counter, evaluate retry policy) as if the worker itself had
reported the failure.

---

# Dead Letter Queue Semantics

Activity is dead-lettered when:

```text
retry_attempts >= max_attempts
```

Engine actions:

* Publish task to DLQ
* Persist failure reason
* Set `dead_lettered_at` on the activity_execution row (status remains
  `FAILED` — dead-lettering is a queue destination and a timestamp, not
  a distinct activity status; see Activity Lifecycle above)

Workflow actions:

* Start compensation
  or
* Fail workflow

Depending on workflow configuration.

---

# Compensation Semantics

## Purpose

Undo completed activities after workflow failure.

Compensation is best-effort rollback.

Compensation is not a database transaction.

---

## Compensation Registration

Each activity may define:

```text
Activity
Compensation Activity
```

Example:

```text
ReserveInventory
ReleaseInventory
```

This mapping is declared per-activity in the workflow definition (see
architecture.md / spec.md schema) rather than inferred at runtime.

---

## Compensation Order

Compensations execute in reverse order.

Example:

```text
ReserveInventory
ChargeCard
CreateShipment
```

Failure:

```text
CreateShipment FAILED
```

Compensation order:

```text
RefundPayment
ReleaseInventory
```

---

## Compensation Flow

Workflow:

```text
RUNNING
   ↓
COMPENSATING
```

Engine actions:

* Determine completed activities
* Reverse order
* Schedule compensations

---

## Compensation Success

All compensations succeed.

Workflow:

```text
COMPENSATING
   ↓
COMPENSATED
```

---

## Compensation Failure

Compensation activity fails.

Retry policy applies (same attempt-counting and backoff semantics as
regular activities — see Retry Semantics above).

If retries exhausted:

Workflow enters:

```text
FAILED
```

Manual intervention is required, and is surfaced through the
`RetryCompensation` API (see spec.md Compensation APIs) rather than being
visible only in logs — an operator can re-trigger compensation for the
affected execution once the underlying issue is resolved.

---

# Cancellation Semantics

## Cancellation Request

User invokes:

```text
CancelWorkflow(execution_id)
```

---

## Pending Workflow

If workflow not started:

```text
PENDING -> CANCELLED
```

---

## Running Workflow

Engine:

* Prevents new activities
* Waits for running activity completion

Then:

```text
RUNNING -> CANCELLED
```

### Race Condition: Late Completion After Cancellation Request

A running activity's success/failure report may arrive **after**
cancellation was requested but **before** the engine has transitioned the
workflow to `CANCELLED`. This is resolved as follows: the late completion
report is persisted as a workflow event (so it remains visible in the
execution history and audit trail), but it is **ignored for workflow
state-machine purposes** — it does not schedule a next activity and does
not prevent or reverse the transition to `CANCELLED`. Cancellation, once
requested, always wins over a late in-flight result.

---

## Compensation on Cancellation

Optional configuration:

```json
{
  "compensate_on_cancel": true
}
```

If enabled:

```text
RUNNING
   ↓
COMPENSATING
```

---

# Idempotency Semantics

## Requirement

Every activity execution receives:

```text
idempotency_key
```

## Key Generation

The key is generated deterministically as:

```text
idempotency_key = execution_id + activity_name + attempt
```

This ensures redeliveries of the *same* attempt (e.g. RabbitMQ
redelivering an unacked message) collide on the same key and are
recognized as duplicates, while a genuine retry — which increments
`attempt` — receives a new key and is allowed to execute.

---

## Duplicate Delivery

Workers may receive same activity multiple times.

Possible causes:

* Worker crash
* Network failure
* RabbitMQ redelivery

---

## Expected Behavior

Repeated execution:

```text
Activity(idempotency_key=X)
```

must produce identical outcome.

No duplicate side effects allowed.

---

# Workflow Event Model

Every state change creates an event.

Examples:

```text
WORKFLOW_STARTED
ACTIVITY_SCHEDULED
ACTIVITY_STARTED
ACTIVITY_COMPLETED
ACTIVITY_FAILED
ACTIVITY_RETRIED
ACTIVITY_DEAD_LETTERED
COMPENSATION_STARTED
COMPENSATION_COMPLETED
WORKFLOW_COMPLETED
WORKFLOW_FAILED
WORKFLOW_CANCELLED
```

---

# Event Persistence

Events are append-only.

Events are never modified.

Events provide:

* Audit trail
* Debugging
* Operational visibility

---

# Crash Recovery

## Workflow Engine Crash

After restart:

* Load persisted state
* Resume execution

No workflow progress lost.

---

## Worker Crash

Activity remains unacknowledged.

RabbitMQ redelivers task.

Activity re-executes.

Idempotency guarantees correctness.

---

## Scheduler Crash

Leader lock expires.

New scheduler acquires leadership.

Schedule processing resumes.

---

# Consistency Guarantees

## Workflow State

Workflow state transitions are strongly consistent.

Implemented through:

* PostgreSQL transactions
* Distributed locking

---

## Activity Delivery

Activity delivery is:

```text
At-Least-Once
```

Not:

```text
Exactly-Once
```

---

## Message Publication

Message publication uses transactional outbox (from Milestone 10
onward; earlier milestones publish directly — see milestones.md).

Guarantee:

```text
Database Commit
and
Message Publication
```

cannot diverge.

---

# Invariants

The following must always hold:

1. A workflow has exactly one active state.
2. A workflow cannot complete unless all activities complete.
3. Only completed activities may be compensated.
4. Compensation order is always reversed.
5. Workflow state changes are persisted before publication.
6. Activity execution records are immutable after completion.
7. Events are append-only.
8. Workflow progress survives process restart.
9. Duplicate activity delivery must not create duplicate side effects.
10. Only one scheduler leader may process schedules at a time.
11. A late-arriving activity result cannot override a workflow that has
    already moved to `CANCELLED`.

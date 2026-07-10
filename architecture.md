# architecture.md

# Cloud-Native Workflow Orchestration Engine Architecture

## Overview

The system consists of loosely coupled services communicating through gRPC, PostgreSQL, RabbitMQ, and Redis.

Design goals:

* Durable workflow execution
* Horizontal scalability
* Fault tolerance
* Service isolation
* Observability
* Operational simplicity

---

# High-Level Architecture

```text
                        ┌─────────────────┐
                        │ Client / UI     │
                        └────────┬────────┘
                                 │
                                 │ REST/gRPC
                                 ▼
                   ┌──────────────────────────┐
                   │ API Service              │
                   └────────────┬─────────────┘
                                │
                                ▼
                   ┌──────────────────────────┐
                   │ Workflow Engine          │
                   └───────┬────────┬─────────┘
                           │        │
                           ▼        ▼

                 PostgreSQL      RabbitMQ
                           ▲        ▲
                           │        │
                  ┌────────┘        └────────┐
                  │                          │
                  ▼                          ▼

        ┌─────────────────┐      ┌─────────────────┐
        │ Scheduler       │      │ Worker Pool     │
        └─────────────────┘      └─────────────────┘

                  ▲
                  │
                  ▼

              Redis
```

Note: Worker Pool reports activity results back to the Workflow Engine
through RabbitMQ (a results queue), not through a direct gRPC call. See
"Activity Result Reporting" under RabbitMQ Topology for the rationale.

---

# Services

## API Service

### Responsibilities

* Expose REST APIs
* Expose gRPC APIs
* Validate requests
* Authenticate requests (future)
* Forward workflow commands

### Stateless

Multiple instances may run simultaneously.

### Dependencies

* Workflow Engine

---

## Workflow Engine

Core orchestration component.

### Responsibilities

* Workflow creation
* Workflow execution state machine
* Activity scheduling
* Retry management
* Compensation execution
* State persistence
* Activity timeout detection (reaper/sweeper)

### Ownership

The Workflow Engine owns workflow state.

No other service modifies workflow state directly.

### Dependencies

* PostgreSQL
* RabbitMQ
* Redis

---

## Scheduler Service

Responsible for scheduled workflows.

### Responsibilities

* Poll due schedules
* Enqueue scheduled workflows
* Maintain schedule state

### Leadership

Only one scheduler instance may actively process schedules.

Leader election is implemented via a **PostgreSQL advisory lock**. This is
the single source of truth for leadership — see "Leader Election" under
Distributed Coordination for the full rationale and Redis's (non-)role.

### Dependencies

* PostgreSQL
* Redis (read-only leadership status cache only — see below)

---

## Worker Service

Executes activities.

### Responsibilities

* Consume activity tasks
* Execute business logic
* Report results
* Emit metrics and traces

### Scaling

Workers are horizontally scalable.

Any number of worker instances may run.

### Dependencies

* RabbitMQ

Results are reported to the Workflow Engine exclusively through RabbitMQ
(a results queue), not via a direct gRPC call. This keeps result delivery
durable and asynchronous, and avoids a double-execution path where a
successful activity's result is lost because a synchronous RPC failed
after the work was already done. The Workflow Engine no longer appears
as a direct runtime dependency of the Worker Service.

---

# Workflow Execution Model

## Workflow Definition

Workflow definitions are stored in PostgreSQL.

Each activity in a definition must declare its compensation activity (if
any), its retry policy overrides, and its timeout. A bare list of activity
names is not sufficient — there is nowhere to express "ChargeCard
compensates with RefundPayment" otherwise.

Example:

```json
{
  "name": "order-processing",
  "activities": [
    {
      "name": "reserve-inventory",
      "compensation": "release-inventory",
      "retry_policy": { "max_attempts": 5, "backoff_strategy": "exponential" },
      "timeout": "30s"
    },
    {
      "name": "charge-card",
      "compensation": "refund-payment",
      "retry_policy": { "max_attempts": 5, "backoff_strategy": "exponential" },
      "timeout": "30s"
    },
    {
      "name": "create-shipment",
      "compensation": "cancel-shipment",
      "retry_policy": { "max_attempts": 3, "backoff_strategy": "exponential" },
      "timeout": "60s"
    }
  ]
}
```

Workflow definitions describe a strictly sequential chain of activities.
Branching, parallel execution, and DAG-shaped workflows are explicitly
out of scope (see spec.md Non-Goals).

---

## Workflow Instance

Each execution creates:

* workflow_execution record
* workflow_event history

Example:

```text
Execution #123

ReserveInventory
      ↓
ChargeCard
      ↓
CreateShipment
```

---

# Workflow State Machine

This diagram is a summary only. **workflow_execution.md is the source of
truth** for the full workflow lifecycle, including `CANCELLED`; refer to
it for the authoritative state machine and transition table.

```text
PENDING
   ↓
RUNNING
   ↓
COMPLETED

or

RUNNING
   ↓
FAILED

or

RUNNING
   ↓
COMPENSATING
   ↓
COMPENSATED

or

PENDING / RUNNING
   ↓
CANCELLED
```

---

# Activity State Machine

This diagram is a summary only. **workflow_execution.md is the source of
truth** for the full activity lifecycle.

```text
PENDING
   ↓
RUNNING
   ↓
COMPLETED

or

RUNNING
   ↓
FAILED
   ↓
RETRYING
   ↓
PENDING
```

Note: dead-lettering is **not** a distinct activity status. An activity
that exhausts its retries remains in the `FAILED` status; a
`dead_lettered_at` timestamp is set and the task is routed to
`activity.dlq`. This avoids a `DLQ` pseudo-state that doesn't fit the
enumerated lifecycle.

---

# Workflow Execution Flow

## Step 1

Client starts workflow.

API Service:

```text
POST /workflows/start
```

---

## Step 2

Workflow Engine:

* Creates execution record
* Creates workflow event
* Schedules first activity

---

## Step 3

Activity task published to RabbitMQ.

Message:

```json
{
  "workflow_id": "...",
  "execution_id": "...",
  "activity": "reserve-inventory",
  "idempotency_key": "..."
}
```

---

## Step 4

Worker consumes task.

Worker:

* Executes activity
* Produces result

---

## Step 5

Worker reports completion via the results queue (see RabbitMQ Topology).

Workflow Engine:

* Persists result
* Updates execution state
* Acknowledges the original task only after the result has been durably
  recorded
* Schedules next activity

---

## Step 6

Continue until workflow completes.

---

# Failure Flow

## Activity Failure

Worker returns failure.

Workflow Engine:

* Increment retry count
* Evaluate retry policy

---

## Retry Available

Workflow Engine:

* Compute backoff
* Requeue activity

---

## Retry Exhausted

Workflow Engine:

* Move task to DLQ (activity status remains `FAILED`; `dead_lettered_at`
  is set — see Activity State Machine)
* Mark workflow failed

If workflow contains compensation handlers:

```text
FAILED
   ↓
COMPENSATING
```

---

# Saga Compensation Flow

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

Compensation:

```text
RefundPayment
ReleaseInventory
```

Execution order:

Reverse of successful activities.

If a compensation activity itself exhausts its retries, the workflow
enters `FAILED` and requires manual intervention. See spec.md's Saga
Compensation section and the `RetryCompensation` API for how an operator
resumes a stuck compensation.

---

# RabbitMQ Topology

## Exchanges

### workflow.exchange

Routes workflow activity tasks.

### results.exchange

Routes activity result reports from Workers back to the Workflow Engine.
Introduced so that result delivery is durable and asynchronous, rather
than a synchronous gRPC call that could fail after the activity's
side effect already succeeded (which would otherwise leave RabbitMQ to
redeliver the unacked task and re-run a "completed" activity).

### dlq.exchange

Routes failed tasks.

## Queues

### Queue-per-activity-type

There is one queue per activity type (e.g. `activity.charge-card.queue`,
`activity.reserve-inventory.queue`), not a single shared `activity.queue`.
This lets worker pools scale independently per activity type — a slow or
high-volume activity (e.g. `charge-card`) can run more worker replicas
without affecting others. The Worker SDK's `RegisterActivity` binds a
handler to the queue matching its activity name.

### activity.results.queue

Workers publish completion/failure reports here; the Workflow Engine
consumes them.

### activity.dlq

Dead letter queue. One per activity type, mirroring the main queues
(`activity.charge-card.dlq`, etc.), so failure inspection and
`RetryFailedTask` can target a specific activity type.

### Delayed retry queues

Native RabbitMQ queues expire messages in FIFO order at the head of the
queue, not per-message. A 16s-delay message sitting behind a 1s-delay
message on the same queue will **not** let the 1s message leave until the
16s message at the head expires. To avoid this, retries use a set of
fixed-TTL "parking" queues, one per backoff tier (`retry.1s.queue`,
`retry.2s.queue`, `retry.4s.queue`, `retry.8s.queue`, `retry.16s.queue`),
each configured with a dead-letter-exchange that routes expired messages
back to the activity's main queue. (Alternative: the
`rabbitmq-delayed-message-exchange` plugin, which supports a per-message
delay natively and avoids the fixed-tier queues — either approach is
acceptable, but the tiered-queue approach is assumed elsewhere in this
document and should be revisited together if the plugin is chosen
instead.)

---

# PostgreSQL Schema

## workflows

Stores workflow definitions.

## workflow_executions

Stores workflow instances.

Fields:

```text
id
workflow_id
status
current_activity
created_at
updated_at
```

---

## workflow_events

Stores execution history.

Fields:

```text
id
execution_id
event_type
payload
timestamp
```

---

## activities

Stores activity metadata, one row per activity declared within a
workflow definition.

Fields:

```text
id
workflow_id
name
compensation_activity_name
retry_policy
timeout
position
```

---

## activity_executions

Stores execution attempts.

Fields:

```text
id
execution_id
activity_name
attempt
status
idempotency_key
started_at
completed_at
dead_lettered_at
```

`idempotency_key` is deterministic and derived as
`execution_id + activity_name + attempt`, so redeliveries of the *same*
attempt collide on the same key, while a genuine retry (which increments
`attempt`) receives a new key.

---

## scheduled_workflows

Stores schedules.

Fields:

```text
id
workflow_id
schedule_type
run_at
cron_expression
interval
next_run_at
status
created_at
updated_at
```

`schedule_type` is one of `once`, `delayed`, `recurring`. `cron_expression`
is used for recurring schedules defined by cron syntax; `interval` is used
for simple fixed-interval recurrence. Exactly one of `run_at`,
`cron_expression`, or `interval` should be populated depending on
`schedule_type`.

---

## outbox

Stores pending message publications.

Fields:

```text
id
aggregate_id
event_type
payload
published_at
created_at
```

Note on rollout: milestones 2–9 publish to RabbitMQ directly from the
Workflow Engine (a real dual-write window exists during this period —
see milestones.md Milestone 4 and Milestone 10). The outbox table and
publisher are introduced in Milestone 10 and replace direct publication
from that point forward.

---

# Outbox Pattern

Used whenever database updates and RabbitMQ publication must occur atomically.

Flow:

```text
BEGIN TRANSACTION

Update Workflow State

Insert Outbox Event

COMMIT
```

Publisher:

```text
Outbox Table
      ↓
Publisher
      ↓
RabbitMQ
```

---

# Distributed Coordination

## Leader Election

Purpose:

Prevent multiple scheduler instances from processing schedules simultaneously.

Implementation:

**PostgreSQL advisory lock.** This is the sole leader-election mechanism —
Redis is not used to hold or arbitrate the lock. (Earlier drafts of this
project's docs left "Postgres lock or Redis lock" as an open choice; it is
now settled in favor of Postgres, since the rest of the system is already
Postgres-consistent and this avoids introducing a second coordination
system.) Redis may still be consulted as a **read-only cache of who is
currently leader**, for cheap status-endpoint reads, but it never holds
the lock itself.

Leader responsibilities:

* Poll schedules
* Enqueue workflows

---

## Distributed Locking

Purpose:

Prevent concurrent workflow state modifications.

Implementation:

PostgreSQL advisory locks.

Lock key:

```text
workflow_execution_id
```

---

# Reliability Patterns

## Idempotency

Every activity execution receives:

```text
idempotency_key
```

Generated deterministically as `execution_id + activity_name + attempt`
(see the `activity_executions` schema above). Workers must safely handle
duplicate deliveries of the same key.

---

## Timeout Detection

Nothing in the message-passing path can detect a timeout on its own — a
worker that crashes mid-activity cannot self-report anything. A
background **reaper/sweeper**, run as part of the Workflow Engine,
periodically scans `activity_executions` for rows stuck in `RUNNING`
status past their declared `timeout`. When found, the reaper marks the
activity `FAILED` (timeout) and hands it to the normal retry-policy
evaluation, exactly as if the worker itself had reported a failure.

This is tied to worker heartbeats (see Worker SDK): a worker sends
periodic heartbeats while executing an activity, and the reaper treats an
activity as a timeout candidate only once both the declared timeout has
elapsed *and* heartbeats have stopped, avoiding false positives on
long-running-but-healthy activities that have their own extended timeout
budget.

---

## Backpressure

Inputs:

* Queue depth
* Worker utilization

Threshold exceeded:

```text
429 Too Many Requests
```

returned by API.

---

## Graceful Shutdown

Shutdown sequence:

```text
Stop accepting requests
      ↓
Finish active work
      ↓
Flush telemetry
      ↓
Close connections
```

---

# Redis Usage

## Leader Election Metadata

Read-only cache of current scheduler leadership, used by status
endpoints. Redis does **not** hold the leader-election lock itself (see
Distributed Coordination above); the lock is a PostgreSQL advisory lock.

## Rate Limiting

Token bucket counters.

## Optional Caching

Workflow definition cache.

---

# Observability

## Metrics

Prometheus metrics exposed by every service.

### Workflow Metrics

```text
workflows_started_total
workflows_completed_total
workflows_failed_total
```

### Activity Metrics

```text
activities_started_total
activities_completed_total
activities_failed_total
activities_retried_total
```

### Queue Metrics

```text
queue_depth
worker_utilization
```

---

## Tracing

OpenTelemetry trace spans:

```text
API Request
      ↓
Workflow Execution
      ↓
Activity Scheduling
      ↓
Worker Execution
      ↓
Activity Completion
```

Trace context propagated through RabbitMQ messages.

---

## Logging

Structured JSON logs.

Required fields:

```json
{
  "trace_id": "",
  "workflow_id": "",
  "execution_id": "",
  "activity_id": "",
  "worker_id": ""
}
```

---

# Deployment Architecture

## Local Development

Docker Compose:

```text
api-service
workflow-engine
scheduler
worker
postgres
rabbitmq
redis
prometheus
grafana
```

Grafana is provisioned with a scrape config against Prometheus; it is not
a separate deliverable with its own milestone (dashboards are optional
polish, not required for milestone acceptance).

---

## Kubernetes

Deployments:

```text
api-service
workflow-engine
scheduler
worker
```

Stateful Services:

```text
postgres
rabbitmq
redis
```

Requirements:

* Liveness probes
* Readiness probes
* Horizontal scaling
* Resource limits

### Liveness vs. Readiness

`/live` must be a pure process check with **no dependency calls**. If
`/live` checked Postgres/RabbitMQ/Redis, a brief blip in any one of them
would cause Kubernetes to kill and restart every pod simultaneously.
`/ready` is where Postgres/RabbitMQ/Redis connectivity is actually
checked; `/health` is a general-purpose combined check for humans/manual
curl, not consumed by Kubernetes probes.

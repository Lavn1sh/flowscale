# milestones.md

# Project Milestones

The project must be implemented incrementally.

Each milestone should result in a working, testable system before proceeding.

---

# Milestone 0: Project Foundation

## Goals

Create repository structure and development environment.

## Tasks

* Initialize Go workspace
* Configure dependency management
* Setup Docker Compose
* Setup PostgreSQL
* Setup basic configuration system
* Setup structured logging
* Setup Makefile
* Setup CI pipeline

## Deliverables

* Repository builds successfully
* Local development environment works
* PostgreSQL accessible

---

# Milestone 1: Workflow Definitions

## Goals

Implement workflow definition management.

## Tasks

* Design database schema
* Create migrations
* Implement workflow CRUD APIs
* Persist workflow definitions
* Validation layer

The schema must support the full per-activity definition shape from day
one — `name`, `compensation`, `retry_policy`, and `timeout` per activity
(see architecture.md / spec.md) — not just a bare list of activity names.
Milestone 8 (Saga) depends on `compensation` already being persisted here.

## Deliverables

Supported operations:

```text
CreateWorkflow
GetWorkflow
ListWorkflows
DeleteWorkflow
```

---

# Milestone 2: Workflow Execution Engine

## Goals

Implement core workflow state machine.

## Tasks

* Create workflow execution tables
* Implement execution state machine
* Implement workflow events
* Persist execution history
* Implement StartWorkflow API

## Deliverables

Workflow execution lifecycle:

```text
PENDING
RUNNING
COMPLETED
FAILED
```

Workflow state survives restart.

---

# Milestone 3: Local Activity Execution

## Goals

Execute activities without RabbitMQ.

## Tasks

* Build worker abstraction
* Register activities
* Execute activities
* Persist activity results
* Update workflow progression

The worker abstraction built here is intentionally minimal — a
`RegisterActivity`/`Start`-shaped interface with no heartbeats, no retry
integration, and no RabbitMQ binding. It is explicitly a throwaway
prototype: Milestone 14 hardens this same interface into the real Worker
SDK (adding heartbeats and retry integration) rather than replacing it
wholesale. Treat this milestone's worker code as scaffolding, not final.

## Deliverables

End-to-end workflow execution.

Example:

```text
ReserveInventory
ChargeCard
CreateShipment
```

runs successfully.

---

# Milestone 4: RabbitMQ Integration

## Goals

Move activity execution to message queues.

## Tasks

* Configure RabbitMQ
* Create exchange topology (`workflow.exchange`, `results.exchange`,
  `dlq.exchange` — see architecture.md)
* Create per-activity-type queues (not a single shared queue)
* Create `activity.results.queue` for worker result reporting
* Publish activity tasks
* Consume activity tasks
* Consume results from `activity.results.queue`; do not ack the original
  task until the result is durably persisted

## Deliverables

Workflow Engine and Workers communicate through RabbitMQ, including
result reporting (no direct gRPC call from Worker to Workflow Engine).

**Note on durability:** this milestone publishes to RabbitMQ directly
from the Workflow Engine's request-handling code — there is a real
dual-write window here (state committed to Postgres but the RabbitMQ
publish fails, or vice versa) that persists through Milestone 9. This is
an accepted incremental-build tradeoff; Milestone 10 replaces this direct
publishing with the transactional outbox pattern.

---

# Milestone 5: Retry Engine

## Goals

Support reliable activity execution.

## Tasks

* Retry policies
* Retry counters
* Exponential backoff
* Retry scheduling via fixed-TTL parking queues per backoff tier (see
  architecture.md RabbitMQ Topology — plain queue TTL expires in FIFO
  order, so a single shared retry queue does not work correctly)

Attempt counting follows workflow_execution.md exactly: `max_attempts: 5`
means 5 total attempts (attempt 1 immediate, attempts 2–5 with backoff
before each), not 5 retries in addition to an initial attempt.

## Deliverables

Failed activities automatically retry.

Example:

```text
Attempt 1
Attempt 2
Attempt 3
Success
```

---

# Milestone 6: Dead Letter Queue

## Goals

Handle permanently failed activities.

## Tasks

* Create DLQ
* Move exhausted retries to DLQ
* DLQ APIs
* Failure inspection

Dead-lettering does not introduce a new activity status. The activity
execution row stays `FAILED`; add and populate a `dead_lettered_at`
timestamp instead (see architecture.md schema and workflow_execution.md
Activity Lifecycle).

## Deliverables

Failed activities appear in DLQ.

Supported operations:

```text
ListFailedTasks
RetryFailedTask
```

---

# Milestone 7: Scheduler

## Goals

Support delayed and recurring workflows.

## Tasks

* Schedule storage (including `cron_expression`/`interval` columns per
  the `scheduled_workflows` schema in architecture.md)
* Scheduler service
* Schedule polling
* Workflow enqueueing

## Deliverables

Supported schedules:

```text
Run at timestamp
Run after delay
Recurring
```

---

# Milestone 8: Saga Compensation

## Goals

Support distributed transaction rollback.

## Tasks

* Compensation registration (already persisted since Milestone 1's
  expanded schema)
* Compensation execution engine
* Reverse execution ordering
* Compensation persistence
* `RetryCompensation` API endpoint, for the case where a compensation
  activity itself exhausts its retries and the workflow is sitting in
  `FAILED` awaiting manual intervention (see spec.md Compensation APIs)

## Deliverables

Failure triggers compensation flow.

Example:

```text
ReserveInventory
ChargeCard
CreateShipment FAILED
```

Compensates:

```text
RefundPayment
ReleaseInventory
```

---

# Milestone 9: Idempotency

## Goals

Prevent duplicate side effects.

## Tasks

* Generate idempotency keys deterministically as
  `execution_id + activity_name + attempt`
* Activity deduplication
* Duplicate detection

## Deliverables

Repeated activity delivery produces single side effect.

---

# Milestone 10: Outbox Pattern

## Goals

Guarantee reliable message publication.

## Tasks

* Create outbox table
* Outbox publisher service
* Message publication workflow
* Recovery handling

**This milestone replaces the direct-publish approach used since
Milestone 4.** From here forward, all database updates that must be
paired with a RabbitMQ publish go through the outbox table instead of
publishing inline; the earlier direct-publish code path should be
removed, not left in place alongside the outbox.

## Deliverables

Database updates and RabbitMQ publication become atomic.

---

# Milestone 11: Reliability Features

## Goals

Improve production readiness.

## Tasks

### Health Checks

Implement:

```text
/health
/live
/ready
```

`/live` must be a pure process check with no dependency calls (no
Postgres/RabbitMQ/Redis pings) — those checks belong on `/ready` only.
This avoids Kubernetes killing every pod simultaneously on a transient
dependency blip.

### Graceful Shutdown

Implement:

* Request draining
* Worker draining
* Connection cleanup

### Backpressure

Implement:

* Queue monitoring
* Worker utilization monitoring
* Request rejection

### Rate Limiting

Implement token bucket algorithm.

## Deliverables

System behaves safely under load and shutdown.

---

# Milestone 12: Distributed Coordination

## Goals

Support multiple scheduler instances.

## Tasks

### Leader Election

Implement:

* PostgreSQL advisory lock

This is the sole leader-election mechanism. (Earlier drafts left this as
"Postgres lock or Redis lock" — that choice is now settled in favor of
Postgres advisory locks, since the rest of the system is already
Postgres-consistent. Redis may still back a read-only "who is currently
leader" cache for status endpoints, but it must not be used to arbitrate
leadership itself.)

### Distributed Locking

Protect workflow state transitions.

## Deliverables

Only one scheduler actively processes schedules.

---

# Milestone 13: Observability

## Goals

Add monitoring and tracing.

## Tasks

### Metrics

Expose:

```text
workflows_started_total
workflows_completed_total
workflows_failed_total

activities_started_total
activities_completed_total
activities_failed_total

queue_depth
worker_utilization
```

### Tracing

Instrument:

* API requests
* Workflow execution
* RabbitMQ messages
* Activity execution

### Logging

Structured JSON logging.

Grafana is provisioned alongside Prometheus (scrape config only) as part
of the Docker Compose / Kubernetes stack; it does not have its own task
list here and is not a required deliverable beyond being reachable and
scraping successfully.

## Deliverables

Metrics visible in Prometheus.

Traces visible through OpenTelemetry.

---

# Milestone 14: Worker SDK

## Goals

Create reusable SDK for activity authors.

## Tasks

* Activity registration
* Context propagation
* Heartbeats
* Retry integration

This milestone hardens the lightweight worker abstraction introduced in
Milestone 3 — adding heartbeats (consumed by the timeout-detection
reaper) and retry integration — rather than building a new interface from
scratch. Activity author code written against the Milestone 3 interface
should need minimal changes to work against this SDK.

## Deliverables

Example:

```go
worker.RegisterActivity(
    "charge-card",
    ChargeCard,
)

worker.Start()
```

---

# Milestone 15: Kubernetes Deployment

## Goals

Deploy platform to Kubernetes.

## Tasks

* Deployment manifests
* Service manifests
* ConfigMaps
* Secrets
* Resource limits
* Readiness probes
* Liveness probes

## Deliverables

All services deploy successfully.

---

# Final Acceptance Criteria

The project is complete when:

* Workflows execute successfully
* Activities execute through RabbitMQ
* Retries work correctly
* DLQ works correctly
* Scheduling works correctly
* Saga compensation works correctly
* Workflow state survives restarts
* Outbox pattern works correctly
* Idempotency works correctly
* Metrics are exposed
* Traces are generated
* Docker deployment works
* Kubernetes deployment works

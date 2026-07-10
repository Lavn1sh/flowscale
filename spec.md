# spec.md

# Cloud-Native Workflow Orchestration Engine

## Overview

A distributed workflow orchestration platform inspired by Temporal and Cadence that enables reliable execution of long-running workflows through task queues, worker pools, retries, scheduling, and saga-based compensation.

The system must provide durable workflow execution, fault tolerance, observability, and horizontal scalability.

---

# Goals

The system must support:

* Workflow definitions
* Workflow execution
* Activity execution
* Task queues
* Worker pools
* Retry policies
* Scheduling
* Saga compensation
* State persistence
* Fault tolerance
* Observability
* Containerized deployment

---

# Non Goals

The following features are out of scope:

* Deterministic workflow replay
* Workflow version migration
* Multi-region replication
* Cross-datacenter failover
* Complex RBAC systems
* Billing systems
* Elasticsearch integration
* CQRS architecture
* Service mesh integration
* Branching, parallel, or DAG-shaped workflows — workflow definitions are
  strictly sequential chains of activities (a single `current_activity`
  per execution). This is an explicit MVP scope limit, not an oversight;
  agents implementing this spec should not attempt fan-out/fan-in,
  conditional branches, or concurrent activity execution within a single
  workflow.

---

# Technology Stack

## Backend

* Go
* gRPC
* REST Gateway

## Storage

* PostgreSQL

## Messaging

* RabbitMQ

## Coordination & Caching

* Redis

## Observability

* OpenTelemetry
* Prometheus
* Grafana

## Infrastructure

* Docker
* Docker Compose
* Kubernetes

---

# Core Concepts

## Workflow

A workflow is a sequence of activities executed according to a workflow definition.

Example:

```text
ReserveInventory
      ↓
ChargeCard
      ↓
CreateShipment
```

---

## Activity

An individual executable task within a workflow.

Examples:

* SendEmail
* ChargeCard
* GenerateInvoice
* UploadFile

Activities must be:

* Retryable
* Idempotent
* Independently executable

---

## Workflow Instance

A running execution of a workflow definition.

Tracks:

* Execution status
* Current activity
* Retry state
* Execution history

---

## Worker

A process that consumes activity tasks from queues and executes them.

Workers can be scaled horizontally.

---

## Task Queue

Durable queue used to distribute activity execution requests to workers.

Implemented using RabbitMQ, with one queue per activity type (see
architecture.md RabbitMQ Topology) so worker pools can scale
independently per activity.

---

# Functional Requirements

## Workflow Definition Management

Support:

* Create workflow definition
* Retrieve workflow definition
* List workflow definitions
* Delete workflow definition

Workflow definition structure. Each activity entry declares its own
compensation activity, retry policy, and timeout — a bare list of names
is not sufficient (see architecture.md for the full rationale and the
`activities` table schema):

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

---

## Workflow Execution

Support:

* Start workflow
* Get workflow status
* List workflow executions
* Cancel workflow

Workflow execution lifecycle:

```text
PENDING
RUNNING
COMPLETED
FAILED
CANCELLED
COMPENSATING
COMPENSATED
```

See workflow_execution.md for the full, authoritative state machine
(including the `COMPENSATING` intermediate state, which this summary
list should be read alongside).

---

## Activity Execution

Activities are executed by workers.

Execution lifecycle:

```text
PENDING
RUNNING
COMPLETED
FAILED
RETRYING
```

Execution result must be persisted.

Dead-lettering is not a separate lifecycle status: an activity that
exhausts its retries remains `FAILED`, with a `dead_lettered_at`
timestamp set (see architecture.md `activity_executions` schema).

---

## Retry Policies

Support configurable retry policies.

Configuration:

```json
{
  "max_attempts": 5,
  "backoff_strategy": "exponential"
}
```

Requirements:

* Retry counters
* Exponential backoff
* Retry timeout handling

Attempt counting is pinned down explicitly to avoid an off-by-one in DLQ
timing: `max_attempts: 5` means **5 total attempts**, not 5 retries after
an initial attempt.

* Attempt 1 runs immediately, with no delay.
* Attempts 2–5 use the backoff table (1s, 2s, 4s, 8s respectively before
  each).
* If attempt 5 fails, the activity is sent to the DLQ — there is no
  attempt 6.

---

## Dead Letter Queue

Activities exceeding retry limits must be moved to a DLQ.

Support:

* List failed tasks
* Retry failed task
* Inspect failure reason

---

## Scheduling

Support workflow scheduling.

Schedule types:

* One-time execution at timestamp
* Delayed execution
* Recurring execution

Scheduler must enqueue workflows when schedules become due.

---

## Saga Compensation

Workflows may define compensation activities.

Example:

```text
ReserveInventory
ChargeCard
CreateShipment
```

Compensations:

```text
ReleaseInventory
RefundPayment
CancelShipment
```

Requirements:

* Execute compensations in reverse order
* Persist compensation state
* Support compensation retries

If a compensation activity itself exhausts its retries, the workflow
enters `FAILED` and requires manual intervention. This is surfaced
through an explicit API rather than only being logged — see the
`RetryCompensation` API under Compensation APIs below.

---

# Persistence

## Database Tables

### workflows

Stores workflow definitions.

### workflow_executions

Stores workflow instances.

### workflow_events

Stores workflow execution history.

### activities

Stores activity metadata, including each activity's compensation
activity name, retry policy, and timeout (see architecture.md schema).

### activity_executions

Stores activity execution attempts, including `idempotency_key` and
`dead_lettered_at` (see architecture.md schema).

### scheduled_workflows

Stores workflow schedules, including `cron_expression`/`interval` for
recurring schedules (see architecture.md schema).

### outbox

Stores pending message publications. Introduced at Milestone 10; earlier
milestones publish to RabbitMQ directly (see milestones.md).

---

# Reliability

## Idempotency

Activities must support idempotent execution.

Requirements:

* Idempotency keys, generated deterministically as
  `execution_id + activity_name + attempt`, so redeliveries of the same
  attempt collide on the same key while a genuine retry (new attempt
  number) gets a new key
* Duplicate execution protection

---

## Outbox Pattern

Message publication must use the transactional outbox pattern.

Flow:

```text
Database Transaction
        ↓
Write Outbox Record
        ↓
Commit
        ↓
Outbox Publisher
        ↓
RabbitMQ
```

---

## Graceful Shutdown

Workers and services must:

* Stop accepting new work
* Finish in-progress work
* Flush telemetry
* Close connections cleanly

---

## Health Checks

Expose:

```text
/health
/ready
/live
```

`/live` is a pure process liveness check with no external dependency
calls — it must not fail due to a transient PostgreSQL/RabbitMQ/Redis
blip, or Kubernetes will restart every pod simultaneously. `/ready`
checks:

* PostgreSQL connectivity
* RabbitMQ connectivity
* Redis connectivity

`/health` is a general-purpose combined check intended for manual/human
inspection, not for Kubernetes probes.

---

## Rate Limiting

Support workflow submission rate limiting.

Implementation:

* Token bucket algorithm

---

## Backpressure

Protect the system during overload.

Inputs:

* Queue depth
* Worker utilization

Actions:

* Reject workflow submissions
* Return retry recommendations

---

# Distributed Coordination

## Leader Election

Required for scheduler instances.

Requirements:

* Single active scheduler
* Automatic failover

Implementation:

**PostgreSQL advisory lock.** (This was previously left open as
"Redis lock or PostgreSQL advisory lock" — it is now settled on Postgres
since the rest of the system is already Postgres-consistent. Redis may
still be used as a read-only cache of current leadership for status
endpoints, but it does not hold the lock.)

---

## Distributed Locking

Protect critical workflow state transitions.

Examples:

* Workflow updates
* Compensation execution

Implementation:

* PostgreSQL advisory locks

---

# Observability

## Metrics

Expose Prometheus metrics.

Workflow metrics:

* workflows_started_total
* workflows_completed_total
* workflows_failed_total

Activity metrics:

* activities_started_total
* activities_completed_total
* activities_failed_total
* activities_retried_total

Queue metrics:

* queue_depth
* worker_utilization

---

## Distributed Tracing

Trace:

* API requests
* Workflow execution
* Activity execution
* RabbitMQ message processing

Implemented using OpenTelemetry.

---

## Logging

Structured JSON logging.

Required fields:

```json
{
  "workflow_id": "",
  "execution_id": "",
  "activity_id": "",
  "worker_id": "",
  "trace_id": ""
}
```

---

# APIs

## Workflow APIs

```text
CreateWorkflow
GetWorkflow
ListWorkflows
DeleteWorkflow
```

---

## Execution APIs

```text
StartWorkflow
GetExecution
ListExecutions
CancelExecution
```

---

## Scheduler APIs

```text
CreateSchedule
ListSchedules
DeleteSchedule
```

---

## DLQ APIs

```text
ListFailedTasks
RetryFailedTask
```

---

## Compensation APIs

```text
RetryCompensation
```

Used when a compensation activity has exhausted its retries and the
workflow is sitting in `FAILED` pending manual intervention. Lets an
operator re-trigger compensation for a specific execution rather than the
failure only being visible in logs.

---

# Worker SDK

Provide a Go SDK for worker implementation.

Capabilities:

* Activity registration
* Task consumption
* Retry handling
* Context propagation
* Heartbeats

Heartbeats are consumed by the Workflow Engine's timeout-detection reaper
(see architecture.md) to distinguish a slow-but-alive activity from one
whose worker crashed mid-execution.

A minimal version of this worker abstraction (registration + start, no
heartbeats yet) is needed starting at Milestone 3, well before the full
SDK is hardened at Milestone 14 — see milestones.md.

Example:

```go
worker.RegisterActivity(
    "charge-card",
    ChargeCard,
)

worker.Start()
```

---

# Deployment

## Docker Compose

Services:

* API Service
* Workflow Engine
* Scheduler
* RabbitMQ
* PostgreSQL
* Redis
* Prometheus
* Grafana

---

## Kubernetes

Deployments:

* API Service
* Workflow Engine
* Scheduler
* Worker

Requirements:

* Liveness probes
* Readiness probes
* Horizontal scaling support

---

# Completion Criteria

The system is complete when:

* Workflow definitions can be managed
* Workflows execute successfully
* Activities are distributed to workers
* Retries function correctly
* DLQ functions correctly
* Scheduling functions correctly
* Saga compensation functions correctly
* Workflow state survives service restarts
* Metrics are exposed
* Traces are generated
* Docker deployment works
* Kubernetes deployment works

# API Documentation

The FlowScale platform provides a RESTful API for managing workflows, executions, schedules, and DLQ.

## Authentication
All API endpoints (except for login and public endpoints) require a Bearer token in the `Authorization` header.

**Endpoint:** `POST /api/auth/login`
- **Request Body:**
  ```json
  {
    "username": "admin",
    "password": "admin"
  }
  ```
- **Response:**
  ```json
  {
    "token": "eyJhb..."
  }
  ```

---

## Workflows

### Create Workflow
**Endpoint:** `POST /workflows`
- **Request Body:**
  ```json
  {
    "name": "OrderProcessing",
    "definition": {
      "activities": [
        { "name": "ReserveInventory", "timeout": "5m" },
        { "name": "ChargeCard", "timeout": "5m", "compensation": "RefundCard" }
      ]
    }
  }
  ```

### List Workflows
**Endpoint:** `GET /workflows`
- **Query Params:**
  - `limit` (default: 50)
  - `offset` (default: 0)

### Delete Workflow
**Endpoint:** `DELETE /workflows/{id}`
Deletes a workflow and all of its associated executions and schedules.

### Start Workflow Execution
**Endpoint:** `POST /workflows/start`
- **Request Body:**
  ```json
  {
    "workflow_id": "workflow-uuid-here"
  }
  ```

---

## Executions

### List Executions
**Endpoint:** `GET /executions`
- **Query Params:**
  - `status` (e.g. `RUNNING`, `FAILED`, `COMPLETED`)
  - `workflow_id`
  - `time_range` (`1h`, `24h`, `7d`)
  - `limit` (default: 50)
  - `offset` (default: 0)

### Get Execution
**Endpoint:** `GET /executions/{id}`

### Get Execution Events
**Endpoint:** `GET /executions/{id}/events`

### Cancel Execution
**Endpoint:** `POST /executions/{id}/cancel`

### Delete Execution
**Endpoint:** `DELETE /executions/{id}`
Permanently removes the execution.

---

## Dead Letter Queue (DLQ)

### List DLQ
**Endpoint:** `GET /activities/dlq`

### Retry DLQ Activity
**Endpoint:** `POST /activities/dlq/retry`
- **Request Body:**
  ```json
  {
    "activity_id": "activity-uuid-here"
  }
  ```

---

## Compensation

### List Compensations
**Endpoint:** `GET /executions/compensation`

### Retry Compensation
**Endpoint:** `POST /executions/{id}/compensation/retry`

---

## Schedules

### Create Schedule
**Endpoint:** `POST /schedules`
- **Request Body:**
  ```json
  {
    "workflow_id": "workflow-uuid-here",
    "cron_expression": "*/5 * * * *"
  }
  ```

### List Schedules
**Endpoint:** `GET /schedules`

### Delete Schedule
**Endpoint:** `DELETE /schedules/{id}`

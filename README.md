# FlowScale

FlowScale is a high-performance, fault-tolerant workflow engine designed to execute long-running distributed processes with durability, retries, saga compensation, and scheduling.

Built as an exploration of workflow engine internals, it tackles the challenges of distributed coordination and asynchronous state execution without relying on existing workflow frameworks.

## Project Structure
- **Backend (Engine)**: Written in Go, it coordinates tasks, persists state in PostgreSQL, and enqueues activities in RabbitMQ.
- **Workers**: A Go SDK to write and execute activities efficiently.
- **API & Core Data**: Complete RESTful interface over engine capabilities.
- **Web UI**: A React-based Single Page Application (SPA) providing a beautiful dashboard for operators.
- **Terminal UI**: A blazing-fast `bubbletea` CLI for terminal power users.

## Features
- **Durable Executions**: Workflows can run for seconds or weeks, surviving restarts without losing state.
- **Retries & DLQ**: Configurable retry policies. Failed activities go to a Dead Letter Queue (DLQ) for manual intervention.
- **Saga Compensation**: Supports long-running transactions by rolling back state automatically via predefined compensation activities.
- **Distributed Coordination**: RabbitMQ handles task dispatch. Scalable by adding more workers.
- **Observability**: OpenTelemetry tracing and structured JSON logging built-in.
- **Cron Scheduling**: Execute recurring workflows on fixed schedules.

## Documentation
Dive deeper into the system internals and API:

- [Getting Started Guide](docs/getting_started.md) - Learn how to run the project locally.
- [API Documentation](docs/api.md) - Complete reference of the REST API endpoints.
- [System Architecture](architecture.md) - Detailed breakdown of components and message flow.
- [Workflow Execution Detail](workflow_execution.md) - Deep dive into how a workflow state machine advances.
- [Milestones & Spec](spec.md) - Project spec and milestones.

## Technologies Used
- **Go** - Backend Engine, Workers, TUI
- **React / TypeScript / Vite** - Web Application
- **PostgreSQL** - Persistence and distributed locking
- **RabbitMQ** - Distributed queues
- **BubbleTea** - Terminal UI

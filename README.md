# FlowScale

FlowScale is a high-performance, fault-tolerant workflow engine designed to execute long-running distributed processes with durability, retries, saga compensation, and scheduling. It tackles the challenges of distributed coordination and asynchronous state execution natively in Go.

![FlowScale Dashboard](./docs/assets/dashboard.png) *(Note: this is a conceptual screenshot of the Web UI)*

## Core Capabilities

- **Durable State Machine**: Workflows can run for seconds or weeks, surviving process restarts or crashes.
- **Horizontal Scalability**: RabbitMQ message broker distributes tasks across independent Worker nodes. Add as many workers as needed.
- **Resilience**: Configurable retries with exponential backoffs, Dead Letter Queues (DLQ) for manual intervention, and automated Saga Compensations for distributed transactions.
- **Cron Scheduling**: Execute recurring workflows on fixed schedules automatically.
- **Multi-Interface**: Interact via a beautiful React web dashboard, a fast Terminal UI (TUI), or a comprehensive REST API.

## Documentation Index

We have comprehensive documentation available in the `/docs` directory:

- [**1. Getting Started**](docs/getting_started.md) - How to spin up FlowScale locally using Docker Compose.
- [**2. Architecture & Design**](docs/architecture.md) - Visual diagrams and explanations of the internal components.
- [**3. How It Works**](docs/how_it_works.md) - Deep dive into workflow execution, the state machine, and scheduling.
- [**4. Clients & Usage**](docs/clients.md) - Guide on using the Web UI, TUI, and HTTP API.
- [**5. Demo Scenarios**](docs/demo_scenarios.md) - Walkthrough of the built-in demos (Heavy Batch, Saga, Retry, etc.).

## Quick Start

You can run the entire distributed stack locally using Docker Compose:

```bash
docker-compose up -d --build
```

Then visit [http://localhost:5173](http://localhost:5173) to view the web application!

## Technologies

- **Backend Engine & Workers**: Go (Golang)
- **Web Dashboard**: React, TypeScript, Vite, TailwindCSS
- **Terminal UI**: BubbleTea (Charm)
- **Data Persistence**: PostgreSQL
- **Message Broker**: RabbitMQ

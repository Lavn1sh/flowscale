# Clients & Interfaces

FlowScale offers multiple ways to interact with the system depending on your preference.

## 1. Web UI (Dashboard)

The Web Dashboard is the primary way to observe the system in production. It is a React SPA built with Vite and TailwindCSS.

### Features
- **Workflows Tab**: View all registered workflows. Use the **"Seed Demos"** button to automatically initialize your database with the built-in demo workflows.
- **Executions Tab**: Real-time monitoring of all active and past workflow runs. Click on an execution to see a step-by-step breakdown (including execution time, states, and errors) of its activities.
- **Schedules Tab**: View and manage Cron schedules.
- **DLQ & Compensations Tab**: View activities that failed terminally. You can manually retry them from here once the underlying issue is resolved.

**Running it**: If using docker-compose, it's available at `http://localhost:5173`.

## 2. Terminal UI (TUI)

For power users, FlowScale includes a lightning-fast terminal dashboard built with Charm's `bubbletea`. 

### Keybindings
- `tab` / `shift+tab`: Navigate between different views (Executions, DLQ, Compensations, Schedules, Workflows).
- `↑` / `↓`: Select items in lists/tables.
- `enter`: View details for a selected item (e.g. view activity steps for an execution).
- `r`: Retry a selected item in the DLQ.
- `c`: Cancel an ongoing execution.
- `d`: Delete a schedule.
- `g`: Seed the database with demo workflows (from the Workflows tab).
- `B`: Batch execute a workflow 10 times concurrently (from the Workflows tab).
- `q` / `ctrl+c`: Quit.

**Running it**: 
Ensure the API is running locally on `localhost:8080`, then run:
```bash
go run ./cmd/tui
```

## 3. HTTP REST API

You can script against FlowScale using standard HTTP requests. The API runs on port `8080`.

- `GET /api/workflows`: List all workflows
- `POST /api/workflows/start`: Start a workflow execution
- `GET /api/executions`: List executions
- `GET /api/executions/{id}`: Get execution details
- `POST /api/activities/dlq/{id}/retry`: Retry a DLQ task
- `GET /api/schedules`: List cron schedules

*Authentication:* Ensure you login via `POST /api/auth/login` to receive your JWT token for protected endpoints.

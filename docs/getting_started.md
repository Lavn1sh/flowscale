# Getting Started

This guide explains how to get FlowScale up and running on your local machine.

## Prerequisites
- **Go 1.22+**
- **Node.js (18+)**
- **Docker & Docker Compose** (for RabbitMQ & PostgreSQL)
- **Make** (optional, but helpful for running commands)

## Step 1: Start Infrastructure
FlowScale requires PostgreSQL for persistence and RabbitMQ for distributed coordination. 
We provide a `docker-compose.yml` to run these dependencies:

```sh
docker-compose up -d
```

## Step 2: Run Database Migrations
Make sure the database schema is up-to-date. The migrations are stored in the `migrations` folder. 
You can use `golang-migrate` to run them:

```sh
migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/flowscale?sslmode=disable" up
```

## Step 3: Run the Engine Backend
The Engine acts as the API Server, Scheduler, and Activity Coordinator.

```sh
# On Linux / macOS (bash)
ROLE=api,scheduler go run ./cmd/engine

# On Windows (PowerShell)
$env:ROLE="api,scheduler"; go run ./cmd/engine
```

## Step 4: Run the Workers
To process the workflow activities, you need to spin up the workers.

```sh
# In a new terminal window
go run ./cmd/worker
```

## Step 5: Start the Web UI
The React Web UI allows you to manage workflows, see executions, and control schedules.

```sh
# In a new terminal window
cd web
npm install
npm run dev
```

You can now visit `http://localhost:5173` in your browser. 
Login using `admin` / `admin`.

## Step 6: (Optional) Run the Terminal UI
If you prefer a fast, keyboard-driven interface, you can run the TUI:

```sh
go run ./cmd/tui
```
Use keys `1` to `5` to switch between views.

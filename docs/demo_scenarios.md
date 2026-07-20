# FlowScale Demo Scenarios

This guide explains how to demonstrate FlowScale's capabilities (Retries, Saga Compensations, DLQ) to a non-technical audience using the Web UI.

The FlowScale engine already has built-in mock workers specifically designed for this! When you run `ROLE=worker go run ./cmd/engine`, the following dummy activities are automatically registered:
- `reserve-inventory` (Success)
- `charge-card` (Success)
- `create-shipment` (Always Fails - to simulate FedEx going down)
- `release-inventory` (Compensation - Success)
- `refund-payment` (Compensation - Success)
- `extract-data` (Success)
- `transform-data` (Success)
- `load-data` (Success)

You can use these to create workflows in the Web UI that visually demonstrate the system's fault tolerance.

---

## 1. The Happy Path Demo (Order Processing)

This workflow demonstrates a successful sequential execution. We will omit the failing `create-shipment` step for this demo.

**In the Web UI, go to "Create Workflow" and use this JSON:**
```json
{
  "name": "ECommerce-HappyPath",
  "definition": {
    "activities": [
      { 
        "name": "reserve-inventory", 
        "timeout": "5m" 
      },
      { 
        "name": "charge-card", 
        "timeout": "5m" 
      }
    ]
  }
}
```
**How to demo:**
1. Start this workflow from the Web UI.
2. Open the "Executions" tab.
3. Show the audience how it sequentially turns green from `reserve-inventory` -> `charge-card`.

---

## 2. Automatic Retries & DLQ Demo

This workflow includes the `create-shipment` activity, which is hardcoded in your Go backend to fail. This allows you to show off the Retry Engine and the Dead Letter Queue.

**In the Web UI, create this Workflow:**
```json
{
  "name": "ECommerce-RetryDemo",
  "definition": {
    "activities": [
      { 
        "name": "reserve-inventory", 
        "timeout": "5m" 
      },
      { 
        "name": "create-shipment", 
        "timeout": "5m",
        "retry_policy": {
          "max_attempts": 3,
          "backoff_coefficient": 2.0,
          "initial_interval": "5s"
        }
      }
    ]
  }
}
```
**How to demo:**
1. In the Web UI header, verify the **Shipment API** toggle is set to 🔴 **DOWN**.
2. Start this workflow.
3. In the Executions tab, show how `reserve-inventory` succeeds.
4. Show how `create-shipment` fails, but the engine **automatically retries** it (you will see the attempt counter go up).
5. After 3 failed attempts, the workflow pauses.
6. Go to the **DLQ** tab in the Web UI. Show the audience the failed task sitting there, waiting for human intervention. 
7. **The Magic Moment**: Explain that the external API has recovered. Toggle the **Shipment API** switch to 🟢 **UP**.
8. Click "Retry" on the failed task in the DLQ. Watch it seamlessly execute and complete the workflow!

---

## 3. Saga Compensation (Rollback) Demo

This demonstrates what happens when a workflow fails fatally and needs to undo previous steps. We will configure compensations so that if the shipment fails, we refund the user and put the item back on the shelf.

**In the Web UI, create this Workflow:**
```json
{
  "name": "ECommerce-SagaDemo",
  "definition": {
    "activities": [
      { 
        "name": "reserve-inventory", 
        "timeout": "5m",
        "compensation": "release-inventory"
      },
      { 
        "name": "charge-card", 
        "timeout": "5m",
        "compensation": "refund-payment"
      },
      { 
        "name": "create-shipment", 
        "timeout": "5m",
        "retry_policy": {
          "max_attempts": 1
        }
      }
    ]
  }
}
```
**How to demo:**
1. Start this workflow.
2. Show the Execution details. It will successfully execute `reserve-inventory` and `charge-card`.
3. When it hits `create-shipment`, it will fail.
4. Because the workflow failed, FlowScale will automatically execute the compensations **in reverse order**.
5. The audience will see `refund-payment` execute, followed by `release-inventory`.
6. Explain: *"Even though the order failed at the end, FlowScale automatically cleaned up our data so the customer was refunded and inventory wasn't lost."*

---

## 4. Scheduled CRON Workflows Demo

This demonstrates how FlowScale can act as a highly reliable distributed cron scheduler (perfect for data synchronization jobs).

**In the Web UI, create this Workflow:**
```json
{
  "name": "Data-Sync-Cron",
  "definition": {
    "activities": [
      { "name": "extract-data", "timeout": "5m" },
      { "name": "transform-data", "timeout": "5m" },
      { "name": "load-data", "timeout": "5m" }
    ]
  }
}
```
**How to demo:**
1. Navigate to the **Schedules** tab.
2. Click **New Schedule**.
3. Set Workflow Name to `Data-Sync-Cron`.
4. Set Cron Expression to `* * * * *` (which means every minute).
5. Click Create.
6. Now, navigate to the **Executions** tab and explain: *"FlowScale will now automatically trigger this workflow reliably every minute."*
7. Wait 60 seconds and show the new executions appearing automatically in the list.

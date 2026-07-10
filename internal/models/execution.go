package models

import (
	"encoding/json"
	"time"
)

type ExecutionStatus string

const (
	ExecutionStatusPending      ExecutionStatus = "PENDING"
	ExecutionStatusRunning      ExecutionStatus = "RUNNING"
	ExecutionStatusCompleted    ExecutionStatus = "COMPLETED"
	ExecutionStatusFailed       ExecutionStatus = "FAILED"
	ExecutionStatusCompensating ExecutionStatus = "COMPENSATING"
	ExecutionStatusCompensated  ExecutionStatus = "COMPENSATED"
	ExecutionStatusCancelled    ExecutionStatus = "CANCELLED"
)

type ActivityStatus string

const (
	ActivityStatusPending   ActivityStatus = "PENDING"
	ActivityStatusRunning   ActivityStatus = "RUNNING"
	ActivityStatusCompleted ActivityStatus = "COMPLETED"
	ActivityStatusFailed    ActivityStatus = "FAILED"
	ActivityStatusRetrying  ActivityStatus = "RETRYING"
)

type EventType string

const (
	EventWorkflowStarted   EventType = "WORKFLOW_STARTED"
	EventWorkflowCompleted EventType = "WORKFLOW_COMPLETED"
	EventWorkflowFailed    EventType = "WORKFLOW_FAILED"
	EventActivityScheduled EventType = "ACTIVITY_SCHEDULED"
	EventActivityCompleted EventType = "ACTIVITY_COMPLETED"
	EventActivityFailed    EventType = "ACTIVITY_FAILED"
)

type WorkflowExecution struct {
	ID              string          `json:"id"`
	WorkflowID      string          `json:"workflow_id"`
	Status          ExecutionStatus `json:"status"`
	CurrentActivity string          `json:"current_activity,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type WorkflowEvent struct {
	ID          string          `json:"id"`
	ExecutionID string          `json:"execution_id"`
	EventType   EventType       `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	Timestamp   time.Time       `json:"timestamp"`
}

type ActivityExecution struct {
	ID             string         `json:"id"`
	ExecutionID    string         `json:"execution_id"`
	ActivityName   string         `json:"activity_name"`
	Attempt        int            `json:"attempt"`
	Status         ActivityStatus `json:"status"`
	IdempotencyKey string         `json:"idempotency_key"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	DeadLetteredAt *time.Time     `json:"dead_lettered_at,omitempty"`
}

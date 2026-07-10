package models

import "time"

const (
	ScheduleTypeOnce      = "once"
	ScheduleTypeDelayed   = "delayed"
	ScheduleTypeRecurring = "recurring"

	ScheduleStatusActive   = "ACTIVE"
	ScheduleStatusFinished = "FINISHED"
)

type Schedule struct {
	ID             string     `json:"id"`
	WorkflowID     string     `json:"workflow_id"`
	ScheduleType   string     `json:"schedule_type"`
	RunAt          *time.Time `json:"run_at,omitempty"`
	CronExpression string     `json:"cron_expression,omitempty"`
	Interval       string     `json:"interval,omitempty"`
	NextRunAt      time.Time  `json:"next_run_at"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type RetryPolicy struct {
	MaxAttempts     int    `json:"max_attempts"`
	BackoffStrategy string `json:"backoff_strategy"`
}

type Activity struct {
	Name         string       `json:"name"`
	Compensation string       `json:"compensation,omitempty"`
	RetryPolicy  *RetryPolicy `json:"retry_policy,omitempty"`
	Timeout      string       `json:"timeout,omitempty"`
}

type Workflow struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Activities []Activity `json:"activities"`
}

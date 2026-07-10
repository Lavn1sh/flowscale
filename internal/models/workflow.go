package models

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

package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WorkflowsStartedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "workflows_started_total",
		Help: "The total number of started workflows",
	})
	WorkflowsCompletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "workflows_completed_total",
		Help: "The total number of completed workflows",
	})
	WorkflowsFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "workflows_failed_total",
		Help: "The total number of failed workflows",
	})

	ActivitiesStartedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "activities_started_total",
		Help: "The total number of started activities",
	})
	ActivitiesCompletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "activities_completed_total",
		Help: "The total number of completed activities",
	})
	ActivitiesFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "activities_failed_total",
		Help: "The total number of failed activities",
	})

	QueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "queue_depth",
		Help: "Current depth of RabbitMQ queues",
	}, []string{"queue"})

	WorkerUtilization = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "worker_utilization",
		Help: "Current number of active goroutines processing tasks in the worker",
	})
)

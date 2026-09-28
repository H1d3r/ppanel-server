package taskqueue

const (
	// ScheduledBatchSendEmail runs a batch email task.
	ScheduledBatchSendEmail = "scheduled:email:batch"

	// ForthwithQuotaTask runs a quota task right away.
	ForthwithQuotaTask = "forthwith:quota:task"

	// SchedulerExchangeRate refreshes the currency exchange rates.
	SchedulerExchangeRate = "scheduler:exchange:rate"
)

package types

const (
	SchedulerCheckSubscription = "scheduler:check:subscription"
	// SchedulerDispatchDomainEvents pumps the generic domain-event outbox
	// onto the asynq queue as events:deliver tasks.
	SchedulerDispatchDomainEvents = "scheduler:events:dispatch"
	SchedulerTotalServerData      = "scheduler:total:server"
	SchedulerResetTraffic         = "scheduler:reset:traffic"
	SchedulerTrafficStat          = "scheduler:traffic:stat"
	SchedulerFlushTraffic         = "scheduler:flush:traffic"
)

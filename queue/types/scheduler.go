package types

const (
	SchedulerCheckSubscription = "scheduler:check:subscription"
	// SchedulerDispatchDomainEvents drains the generic domain-event outbox
	// through the in-process event bus.
	SchedulerDispatchDomainEvents = "scheduler:events:dispatch"
	SchedulerTotalServerData      = "scheduler:total:server"
	SchedulerResetTraffic         = "scheduler:reset:traffic"
	SchedulerTrafficStat          = "scheduler:traffic:stat"
	SchedulerFlushTraffic         = "scheduler:flush:traffic"
)

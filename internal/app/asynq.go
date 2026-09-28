package app

import (
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
)

// queueRedisDB is the Redis database of the task queue. The producer, the
// consumer and the scheduler must agree on it, so it is set here only.
const queueRedisDB = 5

// QueueRedisOpt is the task queue's Redis connection, shared by the task
// client, the consumer and the periodic scheduler.
func QueueRedisOpt(c config.Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: c.Redis.Host, Password: c.Redis.Pass, DB: queueRedisDB}
}

// NewAsynqClient returns the tracing asynq client: EnqueueContext stamps the
// caller's trace context onto the task for the worker-side middleware to
// resume. Pass task options to EnqueueContext, not NewTask — wrapping
// rebuilds the task.
func NewAsynqClient(c config.Config) *taskqueue.Client {
	return taskqueue.NewClient(asynq.NewClient(QueueRedisOpt(c)))
}

func NewAsynqInspector(c config.Config) *asynq.Inspector {
	return asynq.NewInspector(QueueRedisOpt(c))
}

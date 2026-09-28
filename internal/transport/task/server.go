package task

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/pkg/logger"
)

// resetTrafficRetryDelay spaces the calendar traffic reset's retries so a
// database or Redis outage can clear in between; every retry stays on the
// reset's day, which its once-per-day guard is keyed by.
const resetTrafficRetryDelay = 30 * time.Minute

type Service struct {
	deps   Dependencies
	server *asynq.Server
}

// NewService builds the task consumer on the queue's Redis connection; the
// composition root owns that connection's settings.
func NewService(redisOpt asynq.RedisConnOpt, deps Dependencies) *Service {
	return &Service{
		deps:   deps,
		server: initService(redisOpt),
	}
}

func (m *Service) Start() {
	logger.Infof("start consumer service")
	mux := asynq.NewServeMux()
	// Resume the producer's trace from the payload envelope and span every
	// task execution before any handler runs.
	mux.Use(taskqueue.Middleware())
	// register tasks
	RegisterHandlers(mux, m.deps)
	if err := m.server.Run(mux); err != nil {
		logger.Error("consumer service error", logger.LogField{
			Key:   "error",
			Value: err.Error(),
		})
	}
}

func (m *Service) Stop() {
	logger.Info("stop consumer service")
	m.server.Stop()
}

func initService(redisOpt asynq.RedisConnOpt) *asynq.Server {
	return asynq.NewServer(
		redisOpt,
		asynq.Config{
			// The one log line of a failed attempt, naming the task and its
			// retry budget; handlers return their errors without logging them
			// a second time.
			ErrorHandler:   asynq.ErrorHandlerFunc(logTaskFailure),
			RetryDelayFunc: retryDelay,
			Concurrency:    20,
		},
	)
}

func logTaskFailure(ctx context.Context, task *asynq.Task, err error) {
	id, _ := asynq.GetTaskID(ctx)
	retried, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	logger.WithContext(ctx).Errorw("[Task] task failed",
		logger.Field("task_type", task.Type()),
		logger.Field("task_id", id),
		logger.Field("retried", retried),
		logger.Field("max_retry", maxRetry),
		logger.Field("error", err.Error()),
	)
}

func retryDelay(n int, err error, task *asynq.Task) time.Duration {
	if task.Type() == taskqueue.SchedulerResetTraffic {
		return resetTrafficRetryDelay
	}
	return asynq.DefaultRetryDelayFunc(n, err, task)
}

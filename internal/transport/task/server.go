// Package task is the application's asynq consumer: it registers the handler
// of every task type and runs them behind the trace middleware, with one
// failure log line per attempt and the retry policy of the scheduled tasks.
// The handlers live in the subpackages and only decode payloads and call the
// modules, which own the transactions.
package task

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// resetTrafficRetryDelay spaces the calendar traffic reset's retries so a
// database or Redis outage can clear in between; every retry stays on the
// reset's day, which its once-per-day guard is keyed by.
const resetTrafficRetryDelay = 30 * time.Minute

// Service is the task worker the process runs next to the HTTP server and
// the scheduler.
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

// Start registers the handlers and consumes tasks until Stop.
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

// Stop waits for the running handlers and stops consuming.
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

// logTaskFailure logs a failed task run once, with the whole error chain: a
// coded error's message leaves out the failure it wraps, so the log line
// uses xerr.Detail rather than Error.
func logTaskFailure(ctx context.Context, task *asynq.Task, err error) {
	id, _ := asynq.GetTaskID(ctx)
	retried, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	logger.WithContext(ctx).Errorw("[Task] task failed",
		logger.Field("task_type", task.Type()),
		logger.Field("task_id", id),
		logger.Field("retried", retried),
		logger.Field("max_retry", maxRetry),
		logger.Field("error", xerr.Detail(err)),
	)
}

// retryDelay spaces the retries of a failed task: asynq's backoff, except for
// the calendar traffic reset (see resetTrafficRetryDelay).
func retryDelay(n int, err error, task *asynq.Task) time.Duration {
	if task.Type() == taskqueue.SchedulerResetTraffic {
		return resetTrafficRetryDelay
	}
	return asynq.DefaultRetryDelayFunc(n, err, task)
}

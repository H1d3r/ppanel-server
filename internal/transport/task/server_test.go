package task

import (
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
)

// The calendar reset is retried half an hour apart, the other tasks with
// asynq's backoff.
func TestRetryDelay(t *testing.T) {
	failure := errors.New("failed")
	for n := 1; n <= 3; n++ {
		if got := retryDelay(n, failure, asynq.NewTask(taskqueue.SchedulerResetTraffic, nil)); got != resetTrafficRetryDelay {
			t.Fatalf("reset retry %d delay = %v, want %v", n, got, resetTrafficRetryDelay)
		}
		if got := retryDelay(n, failure, asynq.NewTask(taskqueue.SchedulerFlushTraffic, nil)); got <= 0 || got >= resetTrafficRetryDelay {
			t.Fatalf("flush retry %d delay = %v, want asynq's backoff", n, got)
		}
	}
}

package task

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
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

// A failed task's log line keeps the failure a coded error wraps: its Error
// text alone would lose the root cause.
func TestLogTaskFailureKeepsTheWrappedCause(t *testing.T) {
	var output bytes.Buffer
	oldWriter := logger.Reset()
	logger.SetWriter(logger.NewWriter(&output))
	t.Cleanup(func() {
		logger.Reset()
		if oldWriter != nil {
			logger.SetWriter(oldWriter)
		}
	})

	cause := errors.New("connection refused")
	logTaskFailure(context.Background(), asynq.NewTask(taskqueue.SchedulerTrafficStat, nil), xerr.Wrapf(cause, xerr.ERROR, "record the traffic stat"))
	if line := output.String(); !strings.Contains(line, "connection refused") || !strings.Contains(line, "record the traffic stat") {
		t.Fatalf("task failure log = %q, want the context and the cause", line)
	}
}

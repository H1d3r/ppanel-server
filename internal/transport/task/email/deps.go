// Package email holds the queue handlers that send email: single
// notifications (verification codes, expiry, traffic and maintenance notices,
// custom mail) and the marketing campaigns administrators run in batches,
// together with the workers that pace a campaign and record its progress.
package email

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
)

// TaskScheduler enqueues the follow-up task a campaign continues from once
// it reached the daily sending limit; the asynq client provides it.
type TaskScheduler interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Dependencies are the email tasks': the batch tasks' bookkeeping and the
// message log (the platform kernel's), the queue and the runtime settings.
type Dependencies struct {
	Tasks    repository.TaskRepo
	Logs     repository.LogRepo
	Queue    TaskScheduler
	Email    func() config.EmailConfig
	SiteName func() string
}

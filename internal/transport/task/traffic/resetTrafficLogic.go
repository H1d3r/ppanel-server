package traffic

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/transport/task/tasklock"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/redis/go-redis/v9"
)

const (
	resetTrafficLockKey = "reset_traffic_lock"
	resetTrafficLockTTL = 5 * time.Minute
)

// CalendarTrafficResetter is the subscription module's calendar reset.
type CalendarTrafficResetter interface {
	ResetCalendarTraffic(ctx context.Context) error
}

// ResetTrafficLogic is the queue shell of the calendar traffic reset: the
// reset rules and their once-per-day guarantee live in the subscription
// module. A failed run returns its error and asynq retries it (the scheduler
// sets the retry budget); the lock only keeps two runs from overlapping.
type ResetTrafficLogic struct {
	resetter CalendarTrafficResetter
	redis    *redis.Client
}

func NewResetTrafficLogic(resetter CalendarTrafficResetter, rdb *redis.Client) *ResetTrafficLogic {
	return &ResetTrafficLogic{resetter: resetter, redis: rdb}
}

func (l *ResetTrafficLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	lock, ok, err := tasklock.Acquire(ctx, l.redis, resetTrafficLockKey, resetTrafficLockTTL)
	if err != nil {
		return err
	}
	if !ok {
		logger.WithContext(ctx).Info("[ResetTraffic] Another run holds the lock, skipping")
		return nil
	}
	defer releaseLock(ctx, lock, "[ResetTraffic]")
	return l.resetter.ResetCalendarTraffic(ctx)
}

// releaseLock frees a task lock; a failure only delays the next run until
// the lock expires.
func releaseLock(ctx context.Context, lock *tasklock.Lock, tag string) {
	if _, err := lock.Release(ctx); err != nil {
		logger.WithContext(ctx).Errorw(tag+" Release lock failed", logger.Field("error", err.Error()))
	}
}

package traffic

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/transport/task/tasklock"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

const (
	trafficFlushLockKey = "traffic:flush:lock"
	trafficFlushLockTTL = 55 * time.Second
)

type FlushTrafficLogic struct {
	deps Dependencies
}

func NewFlushTrafficLogic(deps Dependencies) *FlushTrafficLogic {
	return &FlushTrafficLogic{deps: deps}
}

func (l *FlushTrafficLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	lock, ok, err := tasklock.Acquire(ctx, l.deps.Redis, trafficFlushLockKey, trafficFlushLockTTL)
	if err != nil {
		return err
	}
	if !ok {
		logger.WithContext(ctx).Info("[FlushTraffic] another task is already running, skipping")
		return nil
	}
	defer releaseLock(ctx, lock, "[FlushTraffic]")
	return network.NewTrafficAggregator(l.deps.Aggregator).FlushDueBuckets(ctx, timeutil.Now())
}

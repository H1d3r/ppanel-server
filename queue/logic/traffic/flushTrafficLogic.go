package traffic

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/internal/trafficagg"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

const trafficFlushLockKey = "traffic:flush:lock"

type FlushTrafficLogic struct {
	svc *svc.ServiceContext
}

func NewFlushTrafficLogic(svc *svc.ServiceContext) *FlushTrafficLogic {
	return &FlushTrafficLogic{svc: svc}
}

func (l *FlushTrafficLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	ok, err := l.svc.Redis.SetNX(ctx, trafficFlushLockKey, "locked", 55*time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		logger.WithContext(ctx).Info("[FlushTraffic] another task is already running, skipping")
		return nil
	}
	defer func() {
		if err := l.svc.Redis.Del(ctx, trafficFlushLockKey).Err(); err != nil {
			logger.WithContext(ctx).Error("[FlushTraffic] release lock failed", logger.Field("error", err.Error()))
		}
	}()

	if err := trafficagg.New(l.svc).FlushDueBuckets(ctx, timeutil.Now()); err != nil {
		logger.WithContext(ctx).Error("[FlushTraffic] flush traffic failed", logger.Field("error", err.Error()))
		return err
	}
	return nil
}

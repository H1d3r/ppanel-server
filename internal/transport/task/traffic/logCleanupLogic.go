package traffic

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

type cleanupStore interface {
	TrafficLog() repository.TrafficRepo
	Log() repository.LogRepo
}

// batchDeleter deletes the rows older than a threshold, one batch per call.
type batchDeleter interface {
	DeleteBeforeBatch(ctx context.Context, end time.Time, limit int) (int64, error)
}

const cleanupBatchSize = 5000

// LogCleanupLogic owns retention independently from traffic aggregation so a
// failed statistics run cannot silently disable log cleanup (or vice versa).
type LogCleanupLogic struct {
	traffic batchDeleter
	logs    batchDeleter
	log     func() config.Log
}

func NewLogCleanupLogic(store cleanupStore, logConfig func() config.Log) *LogCleanupLogic {
	if store == nil {
		return &LogCleanupLogic{log: logConfig}
	}
	return newLogCleanupLogic(store.TrafficLog(), store.Log(), logConfig)
}

func newLogCleanupLogic(traffic, logs batchDeleter, logConfig func() config.Log) *LogCleanupLogic {
	return &LogCleanupLogic{traffic: traffic, logs: logs, log: logConfig}
}

func (l *LogCleanupLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	if l.traffic == nil || l.logs == nil || l.log == nil {
		return nil
	}
	settings := l.log()
	if !settings.AutoClear {
		return nil
	}
	if settings.ClearDays < 1 || settings.ClearDays > 3650 {
		return fmt.Errorf("invalid log retention days: %d", settings.ClearDays)
	}

	now := timeutil.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	threshold := today.AddDate(0, 0, -int(settings.ClearDays))
	trafficDeleted, err := deleteBefore(ctx, l.traffic, threshold)
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "delete the traffic logs before %s", threshold.Format(time.DateOnly))
	}
	logsDeleted, err := deleteBefore(ctx, l.logs, threshold)
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "delete the system logs before %s", threshold.Format(time.DateOnly))
	}
	logger.WithContext(ctx).Infow("[Log Cleanup] cleanup completed", logger.Field("threshold", threshold.Format(time.DateOnly)), logger.Field("traffic_deleted", trafficDeleted), logger.Field("logs_deleted", logsDeleted))
	return nil
}

// deleteBefore deletes the rows older than threshold batch by batch.
func deleteBefore(ctx context.Context, repo batchDeleter, threshold time.Time) (int64, error) {
	var total int64
	for {
		deleted, err := repo.DeleteBeforeBatch(ctx, threshold, cleanupBatchSize)
		total += deleted
		if err != nil || deleted < cleanupBatchSize {
			return total, err
		}
	}
}

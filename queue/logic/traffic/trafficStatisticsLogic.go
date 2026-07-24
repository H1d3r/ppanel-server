package traffic

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/trafficagg"
	"github.com/perfect-panel/server/pkg/logger"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/queue/types"
)

//goland:noinspection GoNameStartsWithPackageName
type TrafficStatisticsLogic struct {
	svc *svc.ServiceContext
}

func NewTrafficStatisticsLogic(svc *svc.ServiceContext) *TrafficStatisticsLogic {
	return &TrafficStatisticsLogic{
		svc: svc,
	}
}

func (l *TrafficStatisticsLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload types.TrafficStatistics
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.WithContext(ctx).Error("[TrafficStatistics] Unmarshal payload failed",
			logger.Field("error", err.Error()),
			logger.Field("payload", string(task.Payload())),
		)
		return nil
	}
	if len(payload.Logs) == 0 {
		logger.WithContext(ctx).Error("[TrafficStatistics] Payload is empty")
		return nil
	}
	// query server info
	serverInfo, err := l.svc.Store.Node().FindOneServer(ctx, payload.ServerId)
	if err != nil {
		logger.WithContext(ctx).Error("[TrafficStatistics] Find server info failed",
			logger.Field("serverId", payload.ServerId),
			logger.Field("error", err.Error()),
		)
		return nil
	}
	if err = trafficagg.New(aggregatorDeps(l.svc)).AddReport(ctx, serverInfo, payload.Protocol, queueTrafficToAggregator(payload.Logs)); err != nil {
		logger.WithContext(ctx).Error("[TrafficStatistics] Aggregate traffic failed",
			logger.Field("serverId", payload.ServerId),
			logger.Field("error", err.Error()),
		)
	}
	return nil
}

func queueTrafficToAggregator(items []types.UserTraffic) []trafficagg.UserTraffic {
	if len(items) == 0 {
		return nil
	}
	result := make([]trafficagg.UserTraffic, 0, len(items))
	for _, item := range items {
		result = append(result, trafficagg.UserTraffic{
			SID:      item.SID,
			Upload:   item.Upload,
			Download: item.Download,
		})
	}
	return result
}

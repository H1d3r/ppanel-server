package server

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/internal/trafficagg"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
)

//goland:noinspection GoNameStartsWithPackageName
type ServerPushUserTrafficLogic struct {
	logger.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewServerPushUserTrafficLogic Push user Traffic
func NewServerPushUserTrafficLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ServerPushUserTrafficLogic {
	return &ServerPushUserTrafficLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ServerPushUserTrafficLogic) ServerPushUserTraffic(req *dto.ServerPushUserTrafficRequest) error {
	// Find server info
	serverInfo, err := l.svcCtx.Store.Node().FindOneServer(l.ctx, req.ServerId)
	if err != nil {
		l.Errorw("[PushOnlineUsers] FindOne error", logger.Field("error", err))
		return errors.New("server not found")
	}

	if err = trafficagg.New(trafficagg.Deps{
		Store: l.svcCtx.Store,
		Redis: l.svcCtx.Redis,
		TrafficReportThreshold: func() int64 {
			return l.svcCtx.Config.Node.TrafficReportThreshold
		},
		Multiplier: func(at time.Time) float32 {
			if l.svcCtx.NodeMultiplierManager == nil {
				return 1
			}
			return l.svcCtx.NodeMultiplierManager.GetMultiplier(at)
		},
	}).AddReport(l.ctx, serverInfo, req.Protocol, dtoTrafficToAggregator(req.Traffic)); err != nil {
		l.Errorw("[ServerPushUserTraffic] Aggregate traffic error", logger.Field("error", err.Error()))
		return errors.Wrap(err, "aggregate traffic")
	}
	return nil
}

func dtoTrafficToAggregator(items []dto.UserTraffic) []trafficagg.UserTraffic {
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

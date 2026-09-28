package traffic

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	trafficEntity "github.com/perfect-panel/server/internal/module/network/entity/traffic"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

type StatLogic struct {
	logs    statLogs
	traffic trafficRankings
}

// statLogs is where the daily statistics are recorded.
type statLogs interface {
	FindFirstByDateType(ctx context.Context, date string, typ uint8) (*log.SystemLog, error)
	InsertBatch(ctx context.Context, data []*log.SystemLog, batchSize int) error
}

// trafficRankings reads a day's traffic per subscription and per server.
type trafficRankings interface {
	QueryUserTrafficRanking(ctx context.Context, start, end time.Time) ([]trafficEntity.UserTrafficRanking, error)
	QueryServerTrafficRanking(ctx context.Context, start, end time.Time) ([]trafficEntity.ServerTrafficRanking, error)
}

func NewStatLogic(deps Dependencies) *StatLogic {
	if deps.Store == nil {
		return &StatLogic{}
	}
	return newStatLogic(deps.Store.Log(), deps.Store.TrafficLog())
}

func newStatLogic(logs statLogs, rankings trafficRankings) *StatLogic {
	return &StatLogic{logs: logs, traffic: rankings}
}

func (l *StatLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	now := timeutil.Now()

	// 获取统计时间范围
	start := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, timeutil.Location())
	end := start.Add(24 * time.Hour)
	date := start.Format(time.DateOnly)

	// The day's rows are written in one atomic batch ending with the stat row,
	// so finding that row means the day is already recorded: a duplicate or
	// replayed run must not insert the rows twice.
	recorded, err := l.logs.FindFirstByDateType(ctx, date, log.TypeTrafficStat.Uint8())
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "query the recorded traffic stat of %s", date)
	}
	if recorded != nil {
		logger.WithContext(ctx).Infof("[Traffic Stat Queue] Traffic of %s already recorded, skipping", date)
		return nil
	}

	// Historical traffic is read outside the write transaction. Once the two
	// aggregate result sets are ready, all daily log rows are persisted with a
	// batched INSERT instead of one INSERT per user/server.
	userTraffic, err := l.traffic.QueryUserTrafficRanking(ctx, start, end)
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "query the user traffic of %s", date)
	}
	serverTraffic, err := l.traffic.QueryServerTrafficRanking(ctx, start, end)
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "query the server traffic of %s", date)
	}

	logs := make([]*log.SystemLog, 0, len(userTraffic)+len(serverTraffic)+3)
	userTop10 := log.UserTrafficRank{Rank: make(map[uint8]log.UserTraffic)}
	stat := log.TrafficStat{}
	for i, trafficData := range userTraffic {
		item := log.UserTraffic{SubscribeId: trafficData.SubscribeId, UserId: trafficData.UserId, Upload: trafficData.Upload, Download: trafficData.Download, Total: trafficData.Total}
		if i < 10 {
			userTop10.Rank[uint8(i+1)] = item
		}
		stat.Upload += item.Upload
		stat.Download += item.Download
		content, _ := item.Marshal()
		logs = append(logs, &log.SystemLog{Type: log.TypeSubscribeTraffic.Uint8(), Date: date, ObjectID: item.SubscribeId, Content: string(content)})
	}
	stat.Total = stat.Upload + stat.Download
	userTop10Content, _ := userTop10.Marshal()
	logs = append(logs, &log.SystemLog{Type: log.TypeUserTrafficRank.Uint8(), Date: date, Content: string(userTop10Content)})

	serverTop10 := log.ServerTrafficRank{Rank: make(map[uint8]log.ServerTraffic)}
	for i, trafficData := range serverTraffic {
		item := log.ServerTraffic{ServerId: trafficData.ServerId, Upload: trafficData.Upload, Download: trafficData.Download, Total: trafficData.Total}
		if i < 10 {
			serverTop10.Rank[uint8(i+1)] = item
		}
		content, _ := item.Marshal()
		logs = append(logs, &log.SystemLog{Type: log.TypeServerTraffic.Uint8(), Date: date, ObjectID: item.ServerId, Content: string(content)})
	}
	serverTop10Content, _ := serverTop10.Marshal()
	logs = append(logs, &log.SystemLog{Type: log.TypeServerTrafficRank.Uint8(), Date: date, Content: string(serverTop10Content)})
	statContent, _ := stat.Marshal()
	logs = append(logs, &log.SystemLog{Type: log.TypeTrafficStat.Uint8(), Date: date, Content: string(statContent)})

	if err := l.logs.InsertBatch(ctx, logs, 1000); err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "record the traffic stat of %s", date)
	}
	logger.WithContext(ctx).Infof("[Traffic Stat Queue] Process task completed successfully, consuming: %s", time.Since(now).String())
	return nil
}

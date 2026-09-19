package auditlog

import (
	"context"
	"time"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type FilterServerTrafficLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewFilterServerTrafficLogLogic Filter server traffic log
func newFilterServerTrafficLogLogic(ctx context.Context, deps Deps) *FilterServerTrafficLogLogic {
	return &FilterServerTrafficLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}
func (l *FilterServerTrafficLogLogic) FilterServerTrafficLog(req *dto.FilterServerTrafficLogRequest) (*dto.FilterServerTrafficLogResponse, error) {
	now := timeutil.Now()
	today := now.Format(time.DateOnly)
	startDate, endDate := req.StartDate, req.EndDate
	if startDate == "" && endDate == "" && req.Date != "" {
		startDate, endDate = req.Date, req.Date
	}
	list := make([]dto.ServerTrafficLog, 0)
	if (startDate == "" || startDate <= today) && (endDate == "" || endDate >= today) {
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, timeutil.Location())
		traffic, err := l.deps.Traffic.QueryServerTrafficRanking(l.ctx, start, start.AddDate(0, 0, 1))
		if err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "today traffic query error: %s", err)
		}
		for _, row := range traffic {
			if req.ServerId != 0 && row.ServerId != req.ServerId {
				continue
			}
			list = append(list, dto.ServerTrafficLog{ServerId: row.ServerId, Upload: row.Upload, Download: row.Download, Total: row.Total, Date: today, Details: true})
		}
	}
	// Today's ranking replaces persisted rows for today, avoiding duplicates.
	yesterday := now.AddDate(0, 0, -1).Format(time.DateOnly)
	if endDate == "" || endDate > yesterday {
		endDate = yesterday
	}
	page, size := repository.NormalizePage(req.Page, req.Size)
	offset := (page - 1) * size
	todayTotal := len(list)
	historyOffset := max(0, offset-todayTotal)
	params := &log.FilterParams{Page: historyOffset/size + 1, Size: size, Type: log.TypeServerTraffic.Uint8(), StartDate: startDate, EndDate: endDate, ObjectID: req.ServerId, Search: req.Search}
	history, historyTotal, err := l.deps.Logs.FilterSystemLog(l.ctx, params)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "history query error: %s", err)
	}
	list = list[min(offset, todayTotal):min(offset+size, todayTotal)]
	// A mixed live/history page can begin partway through a database page.
	skip := historyOffset % size
	history = history[min(skip, len(history)):]
	if len(list)+len(history) < size && int64(historyOffset+len(history)) < historyTotal {
		params.Page++
		params.SkipCount = true
		next, _, err := l.deps.Logs.FilterSystemLog(l.ctx, params)
		if err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "history query error: %s", err)
		}
		history = append(history, next...)
	}
	for _, item := range history {
		if len(list) == size {
			break
		}
		var content log.ServerTraffic
		if err := content.Unmarshal([]byte(item.Content)); err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "corrupt server traffic log %d: %v", item.Id, err)
		}
		hasDetails := true
		if autoClear, clearDays := l.deps.logRetention(); autoClear {
			day, err := time.ParseInLocation(time.DateOnly, item.Date, timeutil.Location())
			hasDetails = err == nil && !day.Before(now.AddDate(0, 0, -int(clearDays)))
		}
		list = append(list, dto.ServerTrafficLog{ServerId: item.ObjectID, Upload: content.Upload, Download: content.Download, Total: content.Total, Date: item.Date, Details: hasDetails})
	}
	return &dto.FilterServerTrafficLogResponse{List: list, Total: int64(todayTotal) + historyTotal}, nil
}

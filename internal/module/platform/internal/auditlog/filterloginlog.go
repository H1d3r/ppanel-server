package auditlog

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type FilterLoginLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewFilterLoginLogLogic Filter login log
func newFilterLoginLogLogic(ctx context.Context, deps Deps) *FilterLoginLogLogic {
	return &FilterLoginLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *FilterLoginLogLogic) FilterLoginLog(req *dto.FilterLoginLogRequest) (resp *dto.FilterLoginLogResponse, err error) {
	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, &log.FilterParams{
		Page:      req.Page,
		Size:      req.Size,
		Type:      log.TypeLogin.Uint8(),
		ObjectID:  req.UserId,
		Data:      req.Date,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Search:    req.Search,
	})

	if err != nil {
		l.Errorf("[FilterLoginLog] failed to filter system log: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "failed to filter system log: %v", err.Error())
	}
	var list []dto.LoginLog
	for _, datum := range data {
		var item log.Login
		err = item.Unmarshal([]byte(datum.Content))
		if err != nil {
			l.Errorf("[FilterLoginLog] failed to unmarshal content: %v", err.Error())
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "corrupt login log %d: %v", datum.Id, err)
		}
		list = append(list, withRequestMetadata(&dto.LoginLog{
			UserId:    datum.ObjectID,
			Method:    item.Method,
			LoginIP:   item.LoginIP,
			Success:   item.Success,
			Timestamp: datum.CreatedAt.UnixMilli(),
		}, requestmeta.Metadata{UserAgent: item.UserAgent, ActorID: item.ActorID, IPMetadata: item.IPMetadata}))
	}

	return &dto.FilterLoginLogResponse{
		Total: total,
		List:  list,
	}, nil
}

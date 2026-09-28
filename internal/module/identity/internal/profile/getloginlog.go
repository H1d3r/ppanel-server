package profile

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetLoginLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Login Log
func newGetLoginLogLogic(ctx context.Context, deps Deps) *GetLoginLogLogic {
	return &GetLoginLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetLoginLogLogic) GetLoginLog(req *dto.GetLoginLogRequest) (resp *dto.GetLoginLogResponse, err error) {
	u, ok := user.FromContext(l.ctx)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, &log.FilterParams{
		Page:     req.Page,
		Size:     req.Size,
		Type:     log.TypeLogin.Uint8(),
		ObjectID: u.Id,
	})
	if err != nil {
		l.Errorw("find login log failed:", logger.Field("error", err.Error()), logger.Field("user_id", u.Id))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find login log failed: %v", err.Error())
	}
	list := make([]dto.UserLoginLog, 0)

	for _, datum := range data {
		var content log.Login
		if err = content.Unmarshal([]byte(datum.Content)); err != nil {
			return nil, xerr.Wrapf(err, xerr.ERROR, "corrupt login log %d", datum.Id)
		}
		list = append(list, dto.UserLoginLog{
			Id:               datum.Id,
			UserId:           datum.ObjectID,
			LoginIP:          content.LoginIP,
			UserAgent:        content.UserAgent,
			Success:          content.Success,
			Timestamp:        datum.CreatedAt.UnixMilli(),
			ActorID:          content.ActorID,
			IPCountryCode:    content.IPCountryCode,
			IPCountry:        content.IPCountry,
			IPRegion:         content.IPRegion,
			IPCity:           content.IPCity,
			IPASN:            content.IPASN,
			IPASOrganization: content.IPASOrganization,
		})
	}

	return &dto.GetLoginLogResponse{
		Total: total,
		List:  list,
	}, nil
}

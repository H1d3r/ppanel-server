package profile

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type GetDeviceListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Device List
func newGetDeviceListLogic(ctx context.Context, deps Deps) *GetDeviceListLogic {
	return &GetDeviceListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetDeviceListLogic) GetDeviceList() (*dto.GetDeviceListResponse, error) {
	userInfo, ok := user.FromContext(l.ctx)
	if !ok {
		return nil, fmt.Errorf("no signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	list, count, err := l.deps.Devices.QueryDeviceList(l.ctx, userInfo.Id)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "list devices of user %d", userInfo.Id)
	}
	userRespList := make([]dto.UserDevice, 0)
	mapping.DeepCopy(&userRespList, list)
	return &dto.GetDeviceListResponse{
		Total: count,
		List:  userRespList,
	}, nil
}

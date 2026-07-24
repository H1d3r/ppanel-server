package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/tool"
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

func (l *GetDeviceListLogic) GetDeviceList() (resp *dto.GetDeviceListResponse, err error) {
	userInfo := l.ctx.Value(constant.CtxKeyUser).(*user.User)
	list, count, err := l.deps.Devices.QueryDeviceList(l.ctx, userInfo.Id)
	userRespList := make([]dto.UserDevice, 0)
	tool.DeepCopy(&userRespList, list)
	resp = &dto.GetDeviceListResponse{
		Total: count,
		List:  userRespList,
	}
	return
}

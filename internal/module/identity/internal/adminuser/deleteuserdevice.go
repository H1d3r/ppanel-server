package adminuser

import (
	"context"

	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/logger"
)

type DeleteUserDeviceLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Delete user device
func newDeleteUserDeviceLogic(ctx context.Context, deps Deps) *DeleteUserDeviceLogic {
	return &DeleteUserDeviceLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *DeleteUserDeviceLogic) DeleteUserDevice(req *dto.DeleteUserDeivceRequest) error {
	err := l.deps.Devices.DeleteDevice(l.ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete user error: %v", err.Error())
	}
	return nil
}

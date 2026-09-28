package profile

import (
	"context"
	"errors"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/devicestate"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

type UnbindDeviceLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Unbind Device
func newUnbindDeviceLogic(ctx context.Context, deps Deps) *UnbindDeviceLogic {
	return &UnbindDeviceLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UnbindDeviceLogic) UnbindDevice(req *dto.UnbindDeviceRequest) error {
	userInfo, ok := user.FromContext(l.ctx)
	if !ok {
		return fmt.Errorf("no signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	device, err := l.deps.Devices.FindDeviceForAuth(l.ctx, req.Id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("device %d does not exist: %w", req.Id, xerr.NewErrCode(xerr.DeviceNotExist))
	}
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find device %d", req.Id)
	}

	if device.UserId != userInfo.Id {
		return fmt.Errorf("device does not belong to the user: %w", xerr.NewErrCode(xerr.InvalidParams))
	}

	removed, err := devicestate.Delete(l.ctx, l.deps.Store, l.deps.Redis, req.Id, userInfo.Id)
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseDeletedError, "remove device %d", req.Id)
	}
	if removed != nil && l.deps.KickDevice != nil {
		l.deps.KickDevice(removed.UserId, removed.Identifier)
	}
	return nil
}

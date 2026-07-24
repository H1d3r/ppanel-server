package profile

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
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
	userInfo := l.ctx.Value(constant.CtxKeyUser).(*user.User)
	device, err := l.deps.Devices.FindOneDevice(l.ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DeviceNotExist), "find device")
	}

	if device.UserId != userInfo.Id {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "device not belong to user")
	}

	return l.deps.Store.InIdentityTx(l.ctx, func(store repository.IdentityStore) error {
		if err = store.UserDevice().DeleteDevice(l.ctx, req.Id); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete device err: %v", err)
		}

		if err = store.UserAuth().DeleteUserAuthMethodByIdentifier(l.ctx, "device", device.Identifier); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find device online record err: %v", err)
		}
		sessionId := l.ctx.Value(constant.CtxKeySessionID)
		sessionIdCacheKey := fmt.Sprintf("%v:%v", config.SessionIdKey, sessionId)
		l.deps.Redis.Del(l.ctx, sessionIdCacheKey)
		return nil
	})
}

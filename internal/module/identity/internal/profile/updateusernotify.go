package profile

import (
	"context"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserNotifyLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update User Notify
func newUpdateUserNotifyLogic(ctx context.Context, deps Deps) *UpdateUserNotifyLogic {
	return &UpdateUserNotifyLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserNotifyLogic) UpdateUserNotify(req *dto.UpdateUserNotifyRequest) error {
	u, ok := user.FromContext(l.ctx)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if u.Id == 0 {
		return fmt.Errorf("no signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	columns := map[string]interface{}{}
	for column, value := range map[string]*bool{
		"enable_login_notify":     req.EnableLoginNotify,
		"enable_balance_notify":   req.EnableBalanceNotify,
		"enable_subscribe_notify": req.EnableSubscribeNotify,
		"enable_trade_notify":     req.EnableTradeNotify,
	} {
		if value != nil {
			columns[column] = *value
		}
	}
	if err := l.deps.Users.UpdateColumns(l.ctx, u.Id, columns); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update notification settings of user %d", u.Id)
	}
	return nil
}

package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/requestctx"
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
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if u.Id == 0 {
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "user not login")
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
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "update user notify error: %v", err.Error())
	}
	return nil
}

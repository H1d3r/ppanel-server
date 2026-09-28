package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserPasswordLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update User Password
func newUpdateUserPasswordLogic(ctx context.Context, deps Deps) *UpdateUserPasswordLogic {
	return &UpdateUserPasswordLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserPasswordLogic) UpdateUserPassword(req *dto.UpdateUserPasswordRequest) error {
	userInfo, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	// A session alone must not be enough to take the account over for good:
	// changing an existing password proves the current one.
	if userInfo.Password != "" && !password.MultiPasswordVerify(userInfo.Algo, userInfo.Salt, req.OldPassword, userInfo.Password) {
		return errors.Wrapf(xerr.NewErrCode(xerr.UserPasswordError), "current password is incorrect")
	}
	// The new hash always uses the current algorithm; a migrated user would
	// otherwise keep verifying it with the old legacy algorithm.
	if err := l.deps.Users.UpdateColumns(l.ctx, userInfo.Id, password.UserColumns(req.Password)); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Update user password error")
	}
	// Every session from before the change ends, including a stolen one.
	if err := usersession.Revoke(l.ctx, l.deps.Redis, userInfo.Id); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "revoke sessions error: %v", err)
	}
	return nil
}

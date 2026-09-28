package adminuser

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type DeleteUserLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newDeleteUserLogic(ctx context.Context, deps Deps) *DeleteUserLogic {
	return &DeleteUserLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *DeleteUserLogic) DeleteUser(req *dto.GetDetailRequest) error {
	if req.Id == demoAdminID && demoMode() {
		return demoRestricted("delete the admin user")
	}
	if err := l.deps.Users.Delete(l.ctx, req.Id); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseDeletedError, "delete user %d", req.Id)
	}
	l.clearDeletedUserAccessCaches([]int64{req.Id})
	return nil
}

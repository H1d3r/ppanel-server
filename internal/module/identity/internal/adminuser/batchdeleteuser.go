package adminuser

import (
	"context"
	"slices"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type BatchDeleteUserLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newBatchDeleteUserLogic(ctx context.Context, deps Deps) *BatchDeleteUserLogic {
	return &BatchDeleteUserLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *BatchDeleteUserLogic) BatchDeleteUser(req *dto.BatchDeleteUserRequest) error {
	if slices.Contains(req.Ids, demoAdminID) && demoMode() {
		return demoRestricted("delete the admin user")
	}
	if err := l.deps.Users.BatchDeleteUser(l.ctx, req.Ids); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseDeletedError, "delete users %v", req.Ids)
	}
	l.clearDeletedUserAccessCaches(req.Ids)
	return nil
}

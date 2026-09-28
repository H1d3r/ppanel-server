package profile

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type LogoutLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Logout ends the calling session
func newLogoutLogic(ctx context.Context, deps Deps) *LogoutLogic {
	return &LogoutLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// Logout ends only the calling session; a password change ends them all.
func (l *LogoutLogic) Logout() error {
	sessionID, _ := l.ctx.Value(requestctx.CtxKeySessionID).(string)
	if sessionID == "" {
		return fmt.Errorf("no session to end: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	if err := usersession.End(l.ctx, l.deps.Redis, sessionID); err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "end session")
	}
	return nil
}

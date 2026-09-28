package profile

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
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
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if err := l.deps.Redis.Del(l.ctx, fmt.Sprintf("%v:%v", config.SessionIdKey, sessionID)).Err(); err != nil {
		l.Errorw("[Logout] delete session failed", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "logout error: %v", err.Error())
	}
	return nil
}

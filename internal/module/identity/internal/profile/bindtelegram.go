package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type BindTelegramLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Bind Telegram
func newBindTelegramLogic(ctx context.Context, deps Deps) *BindTelegramLogic {
	return &BindTelegramLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *BindTelegramLogic) BindTelegram() (resp *dto.BindTelegramResponse, err error) {
	session, ok := l.ctx.Value(constant.CtxKeySessionID).(string)
	if !ok || session == "" {
		l.Errorw("bind telegram failed: session id missing from context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if l.deps.TelegramBotName() == "" {
		l.Errorw("bind telegram failed: telegram bot is not initialized")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "telegram bot is not configured")
	}
	return &dto.BindTelegramResponse{
		Url:       fmt.Sprintf("https://t.me/%s?start=%s", l.deps.TelegramBotName(), session),
		ExpiredAt: timeutil.Now().Add(300 * time.Second).UnixMilli(),
	}, nil
}

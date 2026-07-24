package adminuser

import (
	"context"

	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/tool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type CurrentUserLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newCurrentUserLogic(ctx context.Context, deps Deps) *CurrentUserLogic {
	return &CurrentUserLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *CurrentUserLogic) CurrentUser() (*dto.User, error) {
	resp := &dto.User{}
	u, ok := l.ctx.Value(constant.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}

	l.Logger.Info("current user", zap.Field{Key: "userId", Type: zapcore.Int64Type, Integer: u.Id})
	tool.DeepCopy(resp, u)
	// The context user is the middleware's cached identity row; wallet
	// values come from the billing-owned table.
	if w, err := l.deps.Wallet.FindWallet(l.ctx, u.Id); err == nil && w != nil {
		resp.Balance = w.Balance
		resp.GiftAmount = w.GiftAmount
		resp.Commission = w.Commission
	}
	return resp, nil
}

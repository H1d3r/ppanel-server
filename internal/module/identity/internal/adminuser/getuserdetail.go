package adminuser

import (
	"context"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserDetailLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newGetUserDetailLogic(ctx context.Context, deps Deps) *GetUserDetailLogic {
	return &GetUserDetailLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *GetUserDetailLogic) GetUserDetail(req *dto.GetDetailRequest) (*dto.User, error) {
	resp := dto.User{}
	userInfo, err := l.deps.Users.FindOne(l.ctx, req.Id)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get user detail error: %v", err.Error())
	}
	tool.DeepCopy(&resp, userInfo)
	// Wallet values come from the billing-owned table; the legacy user
	// columns remain only as the dual-written fallback.
	if w, err := l.deps.Wallet.FindWallet(l.ctx, userInfo.Id); err == nil && w != nil {
		resp.Balance = w.Balance
		resp.GiftAmount = w.GiftAmount
		resp.Commission = w.Commission
	}
	return &resp, nil
}

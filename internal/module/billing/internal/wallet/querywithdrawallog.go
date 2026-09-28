package wallet

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type QueryWithdrawalLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewQueryWithdrawalLogLogic Query Withdrawal Log
func newQueryWithdrawalLogLogic(ctx context.Context, deps Deps) *QueryWithdrawalLogLogic {
	return &QueryWithdrawalLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryWithdrawalLogLogic) QueryWithdrawalLog(req *dto.QueryWithdrawalLogListRequest) (resp *dto.QueryWithdrawalLogListResponse, err error) {
	u, ok := user.FromContext(l.ctx)
	if !ok {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "current user is not found in context")
	}
	data, total, err := l.deps.Withdrawals.QueryWithdrawalList(l.ctx, u.Id, nil, req.Page, req.Size)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "query withdrawal log of user %d failed", u.Id)
	}
	list := make([]dto.WithdrawalLog, 0, len(data))
	for _, item := range data {
		list = append(list, withdrawalDTO(item))
	}
	return &dto.QueryWithdrawalLogListResponse{List: list, Total: total}, nil
}

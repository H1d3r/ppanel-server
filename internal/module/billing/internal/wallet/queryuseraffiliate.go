package wallet

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type QueryUserAffiliateLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Query User Balance Log
func newQueryUserAffiliateLogic(ctx context.Context, deps Deps) *QueryUserAffiliateLogic {
	return &QueryUserAffiliateLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryUserAffiliateLogic) QueryUserAffiliate() (resp *dto.QueryUserAffiliateCountResponse, err error) {
	u, ok := user.FromContext(l.ctx)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	total, err := l.deps.Affiliates.CountAffiliates(l.ctx, u.Id)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "Query User Affiliate failed")
	}
	sum, err := l.deps.Logs.SumAmountByTypeAndObjectID(l.ctx, log.TypeCommission.Uint8(), u.Id)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "Query User Affiliate sum commission failed")
	}

	return &dto.QueryUserAffiliateCountResponse{
		Registers:       total,
		TotalCommission: sum,
	}, nil
}

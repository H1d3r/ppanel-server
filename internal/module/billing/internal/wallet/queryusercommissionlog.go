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

type QueryUserCommissionLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Query User Commission Log
func newQueryUserCommissionLogLogic(ctx context.Context, deps Deps) *QueryUserCommissionLogLogic {
	return &QueryUserCommissionLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryUserCommissionLogLogic) QueryUserCommissionLog(req *dto.QueryUserCommissionLogListRequest) (resp *dto.QueryUserCommissionLogListResponse, err error) {
	u, ok := user.FromContext(l.ctx)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, &log.FilterParams{
		Page:     req.Page,
		Size:     req.Size,
		Type:     log.TypeCommission.Uint8(),
		ObjectID: u.Id,
	})
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "Query User Commission Log failed")
	}
	var list []dto.BillingCommissionLogSnapshot

	for _, datum := range data {
		var content log.Commission
		if err = content.Unmarshal([]byte(datum.Content)); err != nil {
			l.Errorf("unmarshal commission log content failed: %v", err.Error())
			continue
		}
		list = append(list, dto.BillingCommissionLogSnapshot{
			UserId: datum.ObjectID,
			Type:   content.Type,
			Amount: content.Amount,
			// OrderNo is left out: it belongs to the referee's order, and
			// subscription tokens issued before random tokens were derived
			// from it.
			Timestamp:        content.Timestamp,
			ClientIP:         content.ClientIP,
			UserAgent:        content.UserAgent,
			ActorID:          content.ActorID,
			IPCountryCode:    content.IPCountryCode,
			IPCountry:        content.IPCountry,
			IPRegion:         content.IPRegion,
			IPCity:           content.IPCity,
			IPASN:            content.IPASN,
			IPASOrganization: content.IPASOrganization,
		})
	}

	return &dto.QueryUserCommissionLogListResponse{
		List:  list,
		Total: total,
	}, nil
}

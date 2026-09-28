// Package userorder implements the user-facing order query subdomain of the
// billing module. Only the module facade may reach it.
package userorder

import (
	"context"
	"errors"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	subscribeEntity "github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	pkgerrors "github.com/pkg/errors"
	"gorm.io/gorm"
)

// PlanReader is the subdomain's read-only port onto the subscription
// domain's plan catalogue: the order rows only carry the plan ID, and the
// plan fields shown on an order detail are attached here instead of through
// a cross-domain SQL association (ADR-001 step 5).
type PlanReader interface {
	FindOne(ctx context.Context, id int64) (*subscribeEntity.Subscribe, error)
}

// Orders reads the current user's orders.
type Orders interface {
	FindOneDetailsByOrderNo(ctx context.Context, orderNo string) (*order.Details, error)
	QueryOrderListByPage(ctx context.Context, page, size int, status uint8, user, subscribe int64, search string) (int64, []*order.Details, error)
}

type Service struct {
	orders Orders
	plans  PlanReader
}

func NewService(orders Orders, plans PlanReader) *Service {
	return &Service{orders: orders, plans: plans}
}

// attachPlan fills the detail's plan fields from the subscription domain.
// A missing plan (deleted, or a recharge order without one) leaves the
// zero value, matching the former SQL association's behaviour.
func (s *Service) attachPlan(ctx context.Context, detail *dto.OrderDetail, cache map[int64]*subscribeEntity.Subscribe) {
	if detail.SubscribeId == 0 || s.plans == nil {
		return
	}
	plan, cached := cache[detail.SubscribeId]
	if !cached {
		found, err := s.plans.FindOne(ctx, detail.SubscribeId)
		if err != nil {
			logger.WithContext(ctx).Errorw("[UserOrder] load plan for order failed",
				logger.Field("error", err.Error()), logger.Field("subscribe_id", detail.SubscribeId))
		} else {
			plan = found
		}
		cache[detail.SubscribeId] = plan
	}
	if plan != nil {
		mapping.DeepCopy(&detail.Subscribe, plan)
	}
}

// QueryDetail returns one of the current user's orders; ownership is
// enforced here and the referrer commission never leaves the module.
func (s *Service) QueryDetail(ctx context.Context, req *dto.QueryOrderDetailRequest) (*dto.OrderDetail, error) {
	currentUser, ok := user.FromContext(ctx)
	if !ok {
		return nil, pkgerrors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	orderInfo, err := s.orders.FindOneDetailsByOrderNo(ctx, req.OrderNo)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, pkgerrors.Wrapf(xerr.NewErrCode(xerr.OrderNotExist), "order %s not found", req.OrderNo)
	}
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find order %s", req.OrderNo)
	}
	if orderInfo.UserId != currentUser.Id {
		return nil, pkgerrors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "order does not belong to the current user")
	}
	resp := &dto.OrderDetail{}
	mapping.DeepCopy(resp, orderInfo)
	s.attachPlan(ctx, resp, map[int64]*subscribeEntity.Subscribe{})
	// Prevent commission amount leakage
	resp.Commission = 0
	return resp, nil
}

func (s *Service) QueryList(ctx context.Context, req *dto.QueryOrderListRequest) (*dto.QueryOrderListResponse, error) {
	u, ok := user.FromContext(ctx)
	if !ok {
		return nil, pkgerrors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	total, data, err := s.orders.QueryOrderListByPage(ctx, req.Page, req.Size, 0, u.Id, 0, "")
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "query orders of user %d", u.Id)
	}
	resp := &dto.QueryOrderListResponse{
		Total: total,
		List:  make([]dto.OrderDetail, 0),
	}
	planCache := map[int64]*subscribeEntity.Subscribe{}
	for _, item := range data {
		var orderInfo dto.OrderDetail
		mapping.DeepCopy(&orderInfo, item)
		s.attachPlan(ctx, &orderInfo, planCache)
		// Prevent commission amount leakage
		orderInfo.Commission = 0
		resp.List = append(resp.List, orderInfo)
	}
	return resp, nil
}

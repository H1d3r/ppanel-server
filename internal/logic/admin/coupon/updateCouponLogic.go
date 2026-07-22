package coupon

import (
	"context"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/coupon"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateCouponLogic struct {
	logger.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// Update coupon
func NewUpdateCouponLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateCouponLogic {
	return &UpdateCouponLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateCouponLogic) UpdateCoupon(req *dto.UpdateCouponRequest) error {
	input := &dto.CreateCouponRequest{
		Name: req.Name, Code: req.Code, Count: req.Count, Type: req.Type,
		Discount: req.Discount, StartTime: req.StartTime, ExpireTime: req.ExpireTime,
		UserLimit: req.UserLimit, Subscribe: req.Subscribe, UsedCount: req.UsedCount, Enable: req.Enable,
	}
	if err := validateCouponInput(input); err != nil {
		return err
	}
	existing, err := l.svcCtx.Store.Coupon().FindOne(l.ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find coupon error: %v", err)
	}
	if req.UsedCount < existing.UsedCount {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "COUPON_USED_COUNT_IMMUTABLE"), "used count cannot be reduced")
	}
	couponInfo := &coupon.Coupon{}
	// update coupon
	tool.DeepCopy(couponInfo, req)
	couponInfo.Subscribe = tool.Int64SliceToString(req.Subscribe)
	if couponInfo.Enable == nil {
		couponInfo.Enable = existing.Enable
	}
	err = l.svcCtx.Store.Coupon().Update(l.ctx, couponInfo)
	if err != nil {
		l.Errorw("[UpdateCoupon] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update coupon error: %v", err.Error())
	}
	return nil
}

package coupon

import (
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func validateCouponInput(req *dto.CreateCouponRequest) error {
	if req.Count < 0 || req.UsedCount < 0 || req.UserLimit < 0 || req.StartTime <= 0 || req.ExpireTime <= req.StartTime {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_COUPON"), "invalid coupon limits or validity window")
	}
	if req.Count > 0 && req.UsedCount > req.Count {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_COUPON"), "used count exceeds coupon count")
	}
	switch req.Type {
	case 1:
		if req.Discount <= 0 || req.Discount > 100 {
			return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_COUPON_DISCOUNT"), "percentage discount must be between 1 and 100")
		}
	case 2:
		if req.Discount <= 0 {
			return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_COUPON_DISCOUNT"), "fixed discount must be positive")
		}
	default:
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_COUPON_TYPE"), "unsupported coupon type")
	}
	return nil
}

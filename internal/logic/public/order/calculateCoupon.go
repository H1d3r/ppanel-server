package order

import (
	"github.com/perfect-panel/server/internal/model/entity/coupon"
	paymentEntity "github.com/perfect-panel/server/internal/model/entity/payment"
	paymentPlatform "github.com/perfect-panel/server/pkg/payment"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func ensureCouponEnabled(couponInfo *coupon.Coupon) error {
	if !couponInfo.IsEnabled() {
		return errors.Wrapf(xerr.NewErrCode(xerr.CouponDisabled), "coupon disabled")
	}
	now := timeutil.Now().Unix()
	if couponInfo.StartTime > 0 && now < couponInfo.StartTime {
		return errors.Wrapf(xerr.NewErrCode(xerr.CouponNotApplicable), "coupon is not active")
	}
	if couponInfo.ExpireTime <= 0 || now > couponInfo.ExpireTime {
		return errors.Wrapf(xerr.NewErrCode(xerr.CouponExpired), "coupon expired")
	}
	return nil
}

func ensurePaymentAvailable(paymentInfo *paymentEntity.Payment) error {
	if paymentInfo == nil || paymentInfo.Enable == nil || !*paymentInfo.Enable || paymentPlatform.ParsePlatform(paymentInfo.Platform) == paymentPlatform.UNSUPPORTED {
		return errors.Wrapf(xerr.NewErrCode(xerr.PaymentMethodNotFound), "payment method is unavailable")
	}
	return nil
}

func calculateCoupon(amount int64, couponInfo *coupon.Coupon) int64 {
	if amount <= 0 || couponInfo == nil || couponInfo.Discount < 0 {
		return 0
	}
	if couponInfo.Type == 1 {
		if couponInfo.Discount > 100 {
			return amount
		}
		return int64(float64(amount) * (float64(couponInfo.Discount) / float64(100)))
	}
	return min(couponInfo.Discount, amount)
}

package pricing

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	pkgerrors "github.com/pkg/errors"
	"gorm.io/gorm"
)

// CouponFinder loads a coupon by its code.
type CouponFinder interface {
	FindOneByCode(ctx context.Context, code string) (*coupon.Coupon, error)
}

// ResolveCoupon loads the coupon an order names and checks that it may
// discount an order for planID at now. An empty code resolves to no coupon.
// The per-user limit is the caller's check: it needs the buyer's orders.
func ResolveCoupon(ctx context.Context, coupons CouponFinder, code string, planID int64, now time.Time) (*coupon.Coupon, error) {
	if code == "" {
		return nil, nil
	}
	found, err := coupons.FindOneByCode(ctx, code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, pkgerrors.Wrapf(xerr.NewErrCode(xerr.CouponNotExist), "coupon not found")
	}
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find coupon %q", code)
	}
	if err := CheckCoupon(found, planID, now); err != nil {
		return nil, err
	}
	return found, nil
}

// CheckCoupon reports why c cannot discount an order for planID at now, or
// nil when it can. Start and expire times are Unix milliseconds.
func CheckCoupon(c *coupon.Coupon, planID int64, now time.Time) error {
	if !c.IsEnabled() {
		return pkgerrors.Wrapf(xerr.NewErrCode(xerr.CouponDisabled), "coupon disabled")
	}
	nowMilli := now.UnixMilli()
	if c.StartTime > 0 && nowMilli < c.StartTime {
		return pkgerrors.Wrapf(xerr.NewErrCode(xerr.CouponNotApplicable), "coupon is not active")
	}
	if c.ExpireTime <= 0 || nowMilli > c.ExpireTime {
		return pkgerrors.Wrapf(xerr.NewErrCode(xerr.CouponExpired), "coupon expired")
	}
	if c.Count != 0 && c.Count <= c.UsedCount {
		return pkgerrors.Wrapf(xerr.NewErrCode(xerr.CouponInsufficientUsage), "coupon used")
	}
	plans, err := slicesx.ParseInt64CSV(c.Subscribe)
	if err != nil {
		// A damaged plan list must not read as an empty one, which would
		// widen the coupon to every plan.
		return xerr.Wrapf(err, xerr.CouponNotApplicable, "coupon %d plan list: %v", c.Id, err)
	}
	if len(plans) > 0 && !slices.Contains(plans, planID) {
		return pkgerrors.Wrapf(xerr.NewErrCode(xerr.CouponNotApplicable), "coupon not match")
	}
	return nil
}

package payment

import (
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

func validatePaymentFee(mode uint, percent, amount int64) error {
	if mode > 3 || percent < 0 || percent > 100 || amount < 0 {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_PAYMENT_FEE"), "invalid payment fee configuration")
	}
	return nil
}

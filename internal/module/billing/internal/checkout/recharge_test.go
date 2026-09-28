package checkout

import (
	"context"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type rechargePayments struct {
	repository.PaymentRepo
	platform string
}

func (p rechargePayments) FindOne(_ context.Context, id int64) (*paymentEntity.Payment, error) {
	enabled := true
	return &paymentEntity.Payment{Id: id, Platform: p.platform, Enable: &enabled}, nil
}

func newRechargeService(platform string) (*Service, *couponLimitOrders) {
	orders := &couponLimitOrders{}
	return NewService(Deps{
		Orders:   orders,
		Payments: rechargePayments{platform: platform},
		Store:    couponLimitStore{tx: couponLimitTx{orders: orders}},
		Queue:    &closeQueue{},
	}), orders
}

// The balance checkout spends gift credit first, so a balance-paid top-up
// would convert gift credit into regular balance.
func TestRechargeRejectsBalancePayment(t *testing.T) {
	svc, orders := newRechargeService("balance")
	_, err := svc.Recharge(ownerContext(42), &dto.RechargeOrderRequest{Amount: 1000, Payment: 1})
	var codeErr *xerr.CodeError
	if !errors.As(errors.Cause(err), &codeErr) || codeErr.GetErrCode() != xerr.PaymentMethodNotFound {
		t.Fatalf("Recharge error = %v, want PaymentMethodNotFound", err)
	}
	if orders.inserted != 0 {
		t.Fatal("a balance-paid recharge order was created")
	}

	svc, orders = newRechargeService("EPay")
	if _, err := svc.Recharge(ownerContext(42), &dto.RechargeOrderRequest{Amount: 1000, Payment: 1}); err != nil {
		t.Fatalf("gateway recharge: %v", err)
	}
	if orders.inserted != 1 {
		t.Fatalf("inserted orders = %d, want 1", orders.inserted)
	}
}

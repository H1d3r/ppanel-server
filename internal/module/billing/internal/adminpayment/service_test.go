package adminpayment

import (
	"context"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/model/dto"
	paymentModel "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type updatePaymentRepo struct {
	repository.PaymentRepo
	stored  *paymentModel.Payment
	updated *paymentModel.Payment
}

func (r *updatePaymentRepo) FindOne(_ context.Context, _ int64) (*paymentModel.Payment, error) {
	return r.stored, nil
}

func (r *updatePaymentRepo) Update(_ context.Context, data *paymentModel.Payment, _ ...*gorm.DB) error {
	r.updated = data
	return nil
}

type updatePaymentOrders struct {
	repository.OrderRepo
	pendingCalls int
}

func (r *updatePaymentOrders) CountPendingByPaymentID(_ context.Context, _ int64) (int64, error) {
	r.pendingCalls++
	return 1, nil
}

// The seeded balance method (config is an empty string) must be toggleable:
// it has no platform config to validate and no callback, so the update must
// neither fail with INVALID_PAYMENT_CONFIG nor hit the pending-order guard.
func TestUpdateBalancePaymentMethodTogglesEnable(t *testing.T) {
	enable := true
	repo := &updatePaymentRepo{stored: &paymentModel.Payment{
		Id:       -1,
		Name:     "Balance",
		Platform: "balance",
		Enable:   new(bool),
	}}
	orders := &updatePaymentOrders{}
	svc := NewService(repo, orders, nil, "", nil)

	_, err := svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
		Id:       -1,
		Name:     "Balance",
		Platform: "balance",
		Config:   map[string]interface{}{},
		Enable:   &enable,
	})
	if err != nil {
		t.Fatalf("Update error = %v, want success", err)
	}
	if repo.updated == nil {
		t.Fatal("payment method was not updated")
	}
	if repo.updated.Enable == nil || !*repo.updated.Enable {
		t.Fatal("Enable was not toggled on")
	}
	if repo.updated.Config != "" {
		t.Fatalf("Config = %q, want stored config preserved", repo.updated.Config)
	}
	if orders.pendingCalls != 0 {
		t.Fatalf("CountPendingByPaymentID calls = %d, want 0 for balance", orders.pendingCalls)
	}
}

// The storefront depends on the seeded balance method (id -1); deleting it
// breaks every balance purchase with a record-not-found at PreCreateOrder.
func TestDeleteBalancePaymentMethodIsRejected(t *testing.T) {
	repo := &updatePaymentRepo{stored: &paymentModel.Payment{
		Id:       -1,
		Name:     "Balance",
		Platform: "balance",
		Enable:   new(bool),
	}}
	orders := &updatePaymentOrders{}
	svc := NewService(repo, orders, nil, "", nil)

	err := svc.Delete(context.Background(), &dto.DeletePaymentMethodRequest{Id: -1})
	if err == nil || !strings.Contains(err.Error(), "cannot be deleted") {
		t.Fatalf("Delete error = %v, want internal-method rejection", err)
	}
}

func TestUpdateGatewayPaymentMethodStillValidatesConfig(t *testing.T) {
	enable := true
	repo := &updatePaymentRepo{stored: &paymentModel.Payment{
		Id:       1,
		Name:     "EPay",
		Platform: "EPay",
		Enable:   new(bool),
	}}
	svc := NewService(repo, &updatePaymentOrders{}, nil, "", nil)

	_, err := svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
		Id:       1,
		Name:     "EPay",
		Platform: "EPay",
		Config:   "not-a-config",
		Enable:   &enable,
	})
	if err == nil {
		t.Fatal("Update error = nil, want INVALID_PAYMENT_CONFIG")
	}
}

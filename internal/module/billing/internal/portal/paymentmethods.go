package portal

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetAvailablePaymentMethods lists the payment methods enabled for checkout.
func (s *Service) GetAvailablePaymentMethods(ctx context.Context) (*dto.GetAvailablePaymentMethodsResponse, error) {
	data, err := s.deps.Payments.FindAvailableMethods(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "list available payment methods")
	}
	resp := &dto.GetAvailablePaymentMethodsResponse{
		List: make([]dto.PaymentMethod, 0),
	}

	mapping.DeepCopy(&resp.List, data)

	return resp, nil
}

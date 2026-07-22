package payment

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/public/payment"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetAvailablePaymentMethodsHandler documents Get available payment methods.
//
// @Summary Get available payment methods
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetAvailablePaymentMethodsResponse}
// @Router /v1/public/payment/methods [get]
func GetAvailablePaymentMethodsHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		l := payment.NewGetAvailablePaymentMethodsLogic(c, svcCtx)
		resp, err := l.GetAvailablePaymentMethods()
		result.HttpResult(ctx, resp, err)
	}
}

package payment

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// CreatePaymentMethodHandler documents Create Payment Method.
//
// @Summary Create Payment Method
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreatePaymentMethodRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.PaymentConfig}
// @Router /v1/admin/payment/ [post]
func CreatePaymentMethodHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.CreatePaymentMethodRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		resp, err := svcCtx.Billing.CreatePaymentMethod(c, &req)
		result.HttpResult(ctx, resp, err)
	}
}

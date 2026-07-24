package payment

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// GetPaymentMethodListHandler documents Get Payment Method List.
//
// @Summary Get Payment Method List
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetPaymentMethodListRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetPaymentMethodListResponse}
// @Router /v1/admin/payment/list [get]
func GetPaymentMethodListHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.GetPaymentMethodListRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		resp, err := svcCtx.Billing.GetPaymentMethodList(c, &req)
		result.HttpResult(ctx, resp, err)
	}
}

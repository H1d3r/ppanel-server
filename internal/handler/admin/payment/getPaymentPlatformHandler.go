package payment

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetPaymentPlatformHandler documents Get supported payment platform.
//
// @Summary Get supported payment platform
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.PlatformResponse}
// @Router /v1/admin/payment/platform [get]
func GetPaymentPlatformHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		resp, err := svcCtx.Billing.GetPaymentPlatform(c)
		result.HttpResult(ctx, resp, err)
	}
}

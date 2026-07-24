package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// GetUserSubscribeDevicesHandler documents Get user subcribe devices.
//
// @Summary Get user subcribe devices
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetUserSubscribeDevicesRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetUserSubscribeDevicesResponse}
// @Router /v1/admin/user/subscribe/device [get]
func GetUserSubscribeDevicesHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.GetUserSubscribeDevicesRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		resp, err := svcCtx.Subscription.GetUserSubscribeDevices(ctx, &req)
		result.HttpResult(c, resp, err)
	}
}

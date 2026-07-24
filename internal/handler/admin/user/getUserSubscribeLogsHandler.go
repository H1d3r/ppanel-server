package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// GetUserSubscribeLogsHandler documents Get user subcribe logs.
//
// @Summary Get user subcribe logs
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetUserSubscribeLogsRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetUserSubscribeLogsResponse}
// @Router /v1/admin/user/subscribe/logs [get]
func GetUserSubscribeLogsHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.GetUserSubscribeLogsRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		resp, err := svcCtx.Subscription.GetUserSubscribeLogs(ctx, &req)
		result.HttpResult(c, resp, err)
	}
}

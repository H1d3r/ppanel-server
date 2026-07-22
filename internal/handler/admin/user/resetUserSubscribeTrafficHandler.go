package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/user"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// ResetUserSubscribeTrafficHandler documents Reset user subscribe traffic.
//
// @Summary Reset user subscribe traffic
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.ResetUserSubscribeTrafficRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/admin/user/subscribe/reset/traffic [post]
func ResetUserSubscribeTrafficHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.ResetUserSubscribeTrafficRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := user.NewResetUserSubscribeTrafficLogic(ctx, svcCtx)
		err := l.ResetUserSubscribeTraffic(&req)
		result.HttpResult(c, nil, err)
	}
}

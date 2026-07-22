package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/public/user"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// GetLoginLogHandler documents Get Login Log.
//
// @Summary Get Login Log
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetLoginLogRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetLoginLogResponse}
// @Router /v1/public/user/login_log [get]
func GetLoginLogHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.GetLoginLogRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		l := user.NewGetLoginLogLogic(c, svcCtx)
		resp, err := l.GetLoginLog(&req)
		result.HttpResult(ctx, resp, err)
	}
}

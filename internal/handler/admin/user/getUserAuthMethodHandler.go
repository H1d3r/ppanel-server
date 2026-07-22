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

// GetUserAuthMethodHandler documents Get user auth method.
//
// @Summary Get user auth method
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetUserAuthMethodRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetUserAuthMethodResponse}
// @Router /v1/admin/user/auth_method [get]
func GetUserAuthMethodHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.GetUserAuthMethodRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := user.NewGetUserAuthMethodLogic(ctx, svcCtx)
		resp, err := l.GetUserAuthMethod(&req)
		result.HttpResult(c, resp, err)
	}
}

package auth

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/auth"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// CheckUserTelephoneHandler documents Check user telephone is exist.
//
// @Summary Check user telephone is exist
// @Tags common
// @Accept json
// @Produce json
// @Param request query dto.TelephoneCheckUserRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.TelephoneCheckUserResponse}
// @Router /v1/auth/check/telephone [get]
func CheckUserTelephoneHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.TelephoneCheckUserRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := auth.NewCheckUserTelephoneLogic(ctx, auth.CheckUserDependencies{Store: svcCtx.Store})
		resp, err := l.CheckUserTelephone(&req)
		result.HttpResult(c, resp, err)
	}
}

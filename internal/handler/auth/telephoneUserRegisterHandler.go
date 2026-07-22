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

// TelephoneUserRegisterHandler documents User Telephone register.
//
// @Summary User Telephone register
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.TelephoneRegisterRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.LoginResponse}
// @Router /v1/auth/register/telephone [post]
func TelephoneUserRegisterHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.TelephoneRegisterRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}
		// get client ip
		req.IP = c.ClientIP()
		req.UserAgent = string(c.UserAgent())
		l := auth.NewTelephoneUserRegisterLogic(ctx, svcCtx)
		resp, err := l.TelephoneUserRegister(&req)
		result.HttpResult(c, resp, err)
	}
}

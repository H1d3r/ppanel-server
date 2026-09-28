package auth

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// ResetPasswordHandler documents Reset password.
//
// The identity service applies the configured Turnstile check, so the
// verify-config argument is no longer read; it stays until the route
// wiring drops it.
//
// @Summary Reset password
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.ResetPasswordRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.LoginResponse}
// @Router /v1/auth/reset [post]
func ResetPasswordHandler(service identity.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.ResetPasswordRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(c, validateErr)
			return
		}
		resp, err := service.ResetPassword(ctx, &req)
		httpx.HttpResult(c, resp, err)
	}
}

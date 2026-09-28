package auth

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// UserLoginHandler documents User login.
//
// The identity service applies the configured Turnstile check, so the
// verify-config argument is no longer read; it stays until the route
// wiring drops it.
//
// @Summary User login
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.UserLoginRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.LoginResponse}
// @Router /v1/auth/login [post]
func UserLoginHandler(service identity.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.UserLoginRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(c, validateErr)
			return
		}

		resp, err := service.UserLogin(ctx, &req)
		httpx.HttpResult(c, resp, err)
	}
}

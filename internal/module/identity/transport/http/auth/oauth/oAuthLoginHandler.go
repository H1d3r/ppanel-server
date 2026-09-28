// Package oauth holds the identity module's handlers of OAuth sign-in: the
// authorization URL, the token exchange and Apple's form-post callback.
package oauth

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// OAuthLoginService is the part of the identity facade OAuthLoginHandler
// calls.
type OAuthLoginService interface {
	OAuthLogin(ctx context.Context, req *dto.OAuthLoginRequest) (*dto.OAuthLoginResponse, error)
}

var _ OAuthLoginService = identity.Service(nil)

// OAuthLoginHandler documents OAuth login.
//
// @Summary OAuth login
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.OAuthLoginRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.OAuthLoginResponse}
// @Router /v1/auth/oauth/login [post]
func OAuthLoginHandler(service OAuthLoginService) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.OAuthLoginRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(c, validateErr)
			return
		}

		resp, err := service.OAuthLogin(ctx, &req)
		httpx.HttpResult(c, resp, err)
	}
}

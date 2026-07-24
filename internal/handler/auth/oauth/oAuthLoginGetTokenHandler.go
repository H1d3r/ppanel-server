package oauth

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// OAuthLoginGetTokenHandler documents OAuth login get token.
//
// @Summary OAuth login get token
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.OAuthLoginGetTokenRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.LoginResponse}
// @Router /v1/auth/oauth/login/token [post]
func OAuthLoginGetTokenHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.OAuthLoginGetTokenRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		resp, err := svcCtx.Identity.OAuthLoginGetToken(ctx, &req, c.ClientIP(), string(c.UserAgent()))
		result.HttpResult(c, resp, err)
	}
}

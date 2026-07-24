package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// BindOAuthCallbackHandler documents Bind OAuth Callback.
//
// @Summary Bind OAuth Callback
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.BindOAuthCallbackRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/public/user/bind_oauth/callback [post]
func BindOAuthCallbackHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.BindOAuthCallbackRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		err := svcCtx.Identity.BindOAuthCallback(c, &req)
		result.HttpResult(ctx, nil, err)
	}
}

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

// UnbindOAuthHandler documents Unbind OAuth.
//
// @Summary Unbind OAuth
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UnbindOAuthRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/public/user/unbind_oauth [post]
func UnbindOAuthHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.UnbindOAuthRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		l := user.NewUnbindOAuthLogic(c, svcCtx)
		err := l.UnbindOAuth(&req)
		result.HttpResult(ctx, nil, err)
	}
}

package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/public/user"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetOAuthMethodsHandler documents Get OAuth Methods.
//
// @Summary Get OAuth Methods
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetOAuthMethodsResponse}
// @Router /v1/public/user/oauth_methods [get]
func GetOAuthMethodsHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		l := user.NewGetOAuthMethodsLogic(c, svcCtx)
		resp, err := l.GetOAuthMethods()
		result.HttpResult(ctx, resp, err)
	}
}

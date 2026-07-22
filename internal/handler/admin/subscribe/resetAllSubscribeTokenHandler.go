package subscribe

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/subscribe"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// ResetAllSubscribeTokenHandler documents Reset all subscribe tokens.
//
// @Summary Reset all subscribe tokens
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.ResetAllSubscribeTokenResponse}
// @Router /v1/admin/subscribe/reset_all_token [post]
func ResetAllSubscribeTokenHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		l := subscribe.NewResetAllSubscribeTokenLogic(c, svcCtx)
		resp, err := l.ResetAllSubscribeToken()
		result.HttpResult(ctx, resp, err)
	}
}

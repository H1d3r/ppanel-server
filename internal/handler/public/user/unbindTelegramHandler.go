package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// UnbindTelegramHandler documents Unbind Telegram.
//
// @Summary Unbind Telegram
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/public/user/unbind_telegram [post]
func UnbindTelegramHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		err := svcCtx.Identity.UnbindTelegram(c)
		result.HttpResult(ctx, nil, err)
	}
}

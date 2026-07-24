package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// SettingTelegramBotHandler documents setting telegram bot.
//
// @Summary setting telegram bot
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/admin/system/setting_telegram_bot [post]
func SettingTelegramBotHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		err := svcCtx.Platform.SettingTelegramBot(ctx)
		result.HttpResult(c, nil, err)
	}
}

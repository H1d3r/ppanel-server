package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/system"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetSubscribeConfigHandler documents Get subscribe config.
//
// @Summary Get subscribe config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.SubscribeConfig}
// @Router /v1/admin/system/subscribe_config [get]
func GetSubscribeConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := system.NewGetSubscribeConfigLogic(ctx, svcCtx)
		resp, err := l.GetSubscribeConfig()
		result.HttpResult(c, resp, err)
	}
}

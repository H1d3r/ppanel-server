package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetTosConfigHandler documents Get Team of Service Config.
//
// @Summary Get Team of Service Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.TosConfig}
// @Router /v1/admin/system/tos_config [get]
func GetTosConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Platform.GetTosConfig(ctx)
		result.HttpResult(c, resp, err)
	}
}

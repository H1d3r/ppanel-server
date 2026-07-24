package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetModuleConfigHandler documents Get Module Config.
//
// @Summary Get Module Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.ModuleConfig}
// @Router /v1/admin/system/module [get]
func GetModuleConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Platform.GetModuleConfig(ctx)
		result.HttpResult(c, resp, err)
	}
}

package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetVerifyConfigHandler documents Get verify config.
//
// @Summary Get verify config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.VerifyConfig}
// @Router /v1/admin/system/verify_config [get]
func GetVerifyConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Platform.GetVerifyConfig(ctx)
		result.HttpResult(c, resp, err)
	}
}

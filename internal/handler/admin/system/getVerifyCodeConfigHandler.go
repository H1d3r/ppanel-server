package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetVerifyCodeConfigHandler documents Get Verify Code Config.
//
// @Summary Get Verify Code Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.VerifyCodeConfig}
// @Router /v1/admin/system/verify_code_config [get]
func GetVerifyCodeConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Platform.GetVerifyCodeConfig(ctx)
		result.HttpResult(c, resp, err)
	}
}

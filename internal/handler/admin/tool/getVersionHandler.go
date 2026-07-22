package tool

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/tool"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetVersionHandler documents Get Version.
//
// @Summary Get Version
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.VersionResponse}
// @Router /v1/admin/tool/version [get]
func GetVersionHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := tool.NewGetVersionLogic(ctx, svcCtx)
		resp, err := l.GetVersion()
		result.HttpResult(c, resp, err)
	}
}

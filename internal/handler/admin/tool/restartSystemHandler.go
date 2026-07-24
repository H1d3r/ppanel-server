package tool

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// RestartSystemHandler documents Restart System.
//
// @Summary Restart System
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/admin/tool/restart [get]
func RestartSystemHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		err := svcCtx.Platform.RestartSystem(ctx)
		result.HttpResult(c, nil, err)
	}
}

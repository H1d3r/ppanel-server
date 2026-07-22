package tool

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/tool"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetSystemLogHandler documents Get System Log.
//
// @Summary Get System Log
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.LogResponse}
// @Router /v1/admin/tool/log [get]
func GetSystemLogHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := tool.NewGetSystemLogLogic(ctx, svcCtx)
		resp, err := l.GetSystemLog()
		result.HttpResult(c, resp, err)
	}
}

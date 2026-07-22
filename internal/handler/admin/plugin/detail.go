package plugin

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/plugin"
	"github.com/perfect-panel/server/internal/svc"
)

// DetailHandler returns loaded plugin details.
//
// @Summary Get loaded plugin details
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name query string true "Plugin name"
// @Success 200 {object} plugin.PluginInfo
// @Router /v1/admin/plugin/detail [get]
func DetailHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		name := ctx.Query("name")

		mgr, ok := svcCtx.PluginMgr.(*plugin.Manager)
		if !ok || mgr == nil {
			ctx.JSON(consts.StatusNotFound, map[string]string{"error": "plugin manager not available"})
			return
		}

		p := mgr.GetPlugin(name)
		if p == nil {
			ctx.JSON(consts.StatusNotFound, map[string]string{"error": "plugin not found"})
			return
		}

		ctx.JSON(consts.StatusOK, p.ToInfo())
	}
}

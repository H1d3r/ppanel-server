package plugin

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/plugin"
	"github.com/perfect-panel/server/internal/svc"
)

// ListHandler lists loaded plugins.
//
// @Summary List loaded plugins
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {array} plugin.PluginInfo
// @Router /v1/admin/plugin/list [get]
func ListHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		mgr, ok := svcCtx.PluginMgr.(*plugin.Manager)
		if !ok || mgr == nil {
			ctx.JSON(consts.StatusOK, []interface{}{})
			return
		}
		plugins := mgr.ListPlugins()
		ctx.JSON(consts.StatusOK, plugins)
	}
}

package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminTool "github.com/perfect-panel/server/internal/handler/admin/tool"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminToolRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminToolGroupRouter := router.Group("/v1/admin/tool")
	adminToolGroupRouter.Use(middleware.AuthMiddleware(serverCtx))
	{
		adminToolGroupRouter.GET("/ip/location", adminTool.QueryIPLocationHandler(serverCtx))
		adminToolGroupRouter.GET("/log", adminTool.GetSystemLogHandler(serverCtx))
		adminToolGroupRouter.GET("/restart", adminTool.RestartSystemHandler(serverCtx))
		adminToolGroupRouter.GET("/version", adminTool.GetVersionHandler(serverCtx))
	}
}

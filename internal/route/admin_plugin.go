package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	pluginHandler "github.com/perfect-panel/server/internal/handler/admin/plugin"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminPluginRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	pluginGroup := router.Group("/v1/admin/plugin")
	pluginGroup.Use(middleware.AuthMiddleware(serverCtx))
	{
		pluginGroup.GET("/list", pluginHandler.ListHandler(serverCtx))
		pluginGroup.GET("/detail", pluginHandler.DetailHandler(serverCtx))
		pluginGroup.POST("/reload", pluginHandler.ReloadHandler(serverCtx))
		pluginGroup.POST("/enable", pluginHandler.EnableHandler(serverCtx))
		pluginGroup.POST("/disable", pluginHandler.DisableHandler(serverCtx))
	}

	pluginsGroup := router.Group("/v1/admin/plugins")
	pluginsGroup.Use(middleware.AuthMiddleware(serverCtx))
	{
		pluginsGroup.GET("", pluginHandler.InstalledListHandler(serverCtx))
		pluginsGroup.GET("/", pluginHandler.InstalledListHandler(serverCtx))
		pluginsGroup.POST("/upload", pluginHandler.InstalledUploadHandler(serverCtx))
		pluginsGroup.POST("/reload-all", pluginHandler.InstalledReloadAllHandler(serverCtx))
		pluginsGroup.GET("/:name", pluginHandler.InstalledDetailHandler(serverCtx))
		pluginsGroup.POST("/:name/validate", pluginHandler.InstalledValidateHandler(serverCtx))
		pluginsGroup.GET("/:name/manifest", pluginHandler.InstalledManifestHandler(serverCtx))
		pluginsGroup.GET("/:name/routes", pluginHandler.InstalledRoutesHandler(serverCtx))
		pluginsGroup.GET("/:name/middlewares", pluginHandler.InstalledMiddlewareHandler(serverCtx))
		pluginsGroup.GET("/:name/events", pluginHandler.InstalledEventsHandler(serverCtx))
		pluginsGroup.GET("/:name/health", pluginHandler.InstalledHealthHandler(serverCtx))
		pluginsGroup.POST("/:name/enable", pluginHandler.InstalledActionHandler(serverCtx, "enable"))
		pluginsGroup.POST("/:name/disable", pluginHandler.InstalledActionHandler(serverCtx, "disable"))
		pluginsGroup.POST("/:name/reload", pluginHandler.InstalledActionHandler(serverCtx, "reload"))
		pluginsGroup.POST("/:name/restart", pluginHandler.InstalledActionHandler(serverCtx, "restart"))
	}
}

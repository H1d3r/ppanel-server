package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminServer "github.com/perfect-panel/server/internal/handler/admin/server"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminServerRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminServerGroupRouter := router.Group("/v1/admin/server")
	adminServerGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create Server
		adminServerGroupRouter.POST("/create", adminServer.CreateServerHandler(serverCtx))

		// Delete Server
		adminServerGroupRouter.POST("/delete", adminServer.DeleteServerHandler(serverCtx))

		// Filter Server List
		adminServerGroupRouter.GET("/list", adminServer.FilterServerListHandler(serverCtx))

		// Create Node
		adminServerGroupRouter.POST("/node/create", adminServer.CreateNodeHandler(serverCtx))

		// Delete Node
		adminServerGroupRouter.POST("/node/delete", adminServer.DeleteNodeHandler(serverCtx))

		// Filter Node List
		adminServerGroupRouter.GET("/node/list", adminServer.FilterNodeListHandler(serverCtx))

		// Reset node sort
		adminServerGroupRouter.POST("/node/sort", adminServer.ResetSortWithNodeHandler(serverCtx))

		// Toggle Node Status
		adminServerGroupRouter.POST("/node/status/toggle", adminServer.ToggleNodeStatusHandler(serverCtx))

		// Query all node tags
		adminServerGroupRouter.GET("/node/tags", adminServer.QueryNodeTagHandler(serverCtx))

		// Get Server Node Config
		adminServerGroupRouter.GET("/node_config", adminServer.GetServerNodeConfigHandler(serverCtx))

		// Update Server Node Config
		adminServerGroupRouter.POST("/node_config/update", adminServer.UpdateServerNodeConfigHandler(serverCtx))

		// Update Node
		adminServerGroupRouter.POST("/node/update", adminServer.UpdateNodeHandler(serverCtx))

		// Get Server Protocols
		adminServerGroupRouter.GET("/protocols", adminServer.GetServerProtocolsHandler(serverCtx))

		// Reset server sort
		adminServerGroupRouter.POST("/server/sort", adminServer.ResetSortWithServerHandler(serverCtx))

		// Update Server
		adminServerGroupRouter.POST("/update", adminServer.UpdateServerHandler(serverCtx))
	}
}

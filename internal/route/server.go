package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	serverHandler "github.com/perfect-panel/server/internal/handler/server"
	"github.com/perfect-panel/server/internal/svc"
)

func registerServerRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	serverGroup := router.Group("/v1/server", serverHandler.ServerMiddleware(serverCtx))
	serverGroup.GET("/config", serverHandler.GetServerConfigHandler(serverCtx))
	serverGroup.POST("/online", serverHandler.PushOnlineUsersHandler(serverCtx))
	serverGroup.POST("/push", serverHandler.ServerPushUserTrafficHandler(serverCtx))
	serverGroup.POST("/status", serverHandler.ServerPushStatusHandler(serverCtx))
	serverGroup.GET("/user", serverHandler.GetServerUserListHandler(serverCtx))

	router.GET("/v2/server/:server_id", serverHandler.QueryServerProtocolConfigHandler(serverCtx))
}

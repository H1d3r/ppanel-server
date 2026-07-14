package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicSubscribe "github.com/perfect-panel/server/internal/handler/public/subscribe"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicSubscribeRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicSubscribeGroupRouter := router.Group("/v1/public/subscribe")
	publicSubscribeGroupRouter.Use(middleware.AuthMiddleware(serverCtx), middleware.DeviceMiddleware(serverCtx))
	publicSubscribeGroupRouter.GET("/list", publicSubscribe.QuerySubscribeListHandler(serverCtx))
	publicSubscribeGroupRouter.GET("/node/list", publicSubscribe.QueryUserSubscribeNodeListHandler(serverCtx))
}

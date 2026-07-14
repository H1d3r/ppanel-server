package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/perfect-panel/server/internal/handler"
	"github.com/perfect-panel/server/internal/svc"
)

func registerSubscribeConfigRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	subscribePath := serverCtx.Config.Subscribe.SubscribePath
	if subscribePath == "" {
		subscribePath = "/v1/subscribe/config"
	}
	router.GET(subscribePath, handler.SubscribeHandler(serverCtx))
	if serverCtx.Config.Subscribe.PanDomain {
		router.GET("/", handler.PanDomainSubscribeHandler(serverCtx))
	}
}

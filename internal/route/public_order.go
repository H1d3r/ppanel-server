package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicOrder "github.com/perfect-panel/server/internal/handler/public/order"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicOrderRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicOrderGroupRouter := router.Group("/v1/public/order")
	publicOrderGroupRouter.Use(middleware.AuthMiddleware(serverCtx), middleware.DeviceMiddleware(serverCtx))
	publicOrderGroupRouter.POST("/close", publicOrder.CloseOrderHandler(serverCtx))
	publicOrderGroupRouter.GET("/detail", publicOrder.QueryOrderDetailHandler(serverCtx))
	publicOrderGroupRouter.GET("/list", publicOrder.QueryOrderListHandler(serverCtx))
	publicOrderGroupRouter.POST("/pre", publicOrder.PreCreateOrderHandler(serverCtx))
	publicOrderGroupRouter.POST("/purchase", publicOrder.PurchaseHandler(serverCtx))
	publicOrderGroupRouter.POST("/recharge", publicOrder.RechargeHandler(serverCtx))
	publicOrderGroupRouter.POST("/renewal", publicOrder.RenewalHandler(serverCtx))
	publicOrderGroupRouter.POST("/reset", publicOrder.ResetTrafficHandler(serverCtx))
}

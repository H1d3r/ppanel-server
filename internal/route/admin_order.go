package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminOrder "github.com/perfect-panel/server/internal/handler/admin/order"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminOrderRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminOrderGroupRouter := router.Group("/v1/admin/order")
	adminOrderGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create order
		adminOrderGroupRouter.POST("/", adminOrder.CreateOrderHandler(serverCtx))

		// Get order list
		adminOrderGroupRouter.GET("/list", adminOrder.GetOrderListHandler(serverCtx))

		// Update order status
		adminOrderGroupRouter.PUT("/status", adminOrder.UpdateOrderStatusHandler(serverCtx))
	}
}

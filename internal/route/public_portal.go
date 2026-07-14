package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicPortal "github.com/perfect-panel/server/internal/handler/public/portal"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicPortalRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicPortalGroupRouter := router.Group("/v1/public/portal")
	publicPortalGroupRouter.Use(middleware.DeviceMiddleware(serverCtx))
	publicPortalGroupRouter.POST("/order/checkout", publicPortal.PurchaseCheckoutHandler(serverCtx))
	publicPortalGroupRouter.GET("/order/status", publicPortal.QueryPurchaseOrderHandler(serverCtx))
	publicPortalGroupRouter.GET("/payment-method", publicPortal.GetAvailablePaymentMethodsHandler(serverCtx))
	publicPortalGroupRouter.POST("/pre", publicPortal.PrePurchaseOrderHandler(serverCtx))
	publicPortalGroupRouter.POST("/purchase", publicPortal.PurchaseHandler(serverCtx))
	publicPortalGroupRouter.GET("/subscribe", publicPortal.GetSubscriptionHandler(serverCtx))
}

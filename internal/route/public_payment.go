package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicPayment "github.com/perfect-panel/server/internal/handler/public/payment"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicPaymentRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicPaymentGroupRouter := router.Group("/v1/public/payment")
	publicPaymentGroupRouter.Use(middleware.AuthMiddleware(serverCtx), middleware.DeviceMiddleware(serverCtx))
	publicPaymentGroupRouter.GET("/methods", publicPayment.GetAvailablePaymentMethodsHandler(serverCtx))
}

package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminPayment "github.com/perfect-panel/server/internal/handler/admin/payment"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminPaymentRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminPaymentGroupRouter := router.Group("/v1/admin/payment")
	adminPaymentGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create Payment Method
		adminPaymentGroupRouter.POST("/", adminPayment.CreatePaymentMethodHandler(serverCtx))

		// Update Payment Method
		adminPaymentGroupRouter.PUT("/", adminPayment.UpdatePaymentMethodHandler(serverCtx))

		// Delete Payment Method
		adminPaymentGroupRouter.DELETE("/", adminPayment.DeletePaymentMethodHandler(serverCtx))

		// Get Payment Method List
		adminPaymentGroupRouter.GET("/list", adminPayment.GetPaymentMethodListHandler(serverCtx))

		// Get supported payment platform
		adminPaymentGroupRouter.GET("/platform", adminPayment.GetPaymentPlatformHandler(serverCtx))
	}
}

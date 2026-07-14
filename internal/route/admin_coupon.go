package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminCoupon "github.com/perfect-panel/server/internal/handler/admin/coupon"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminCouponRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminCouponGroupRouter := router.Group("/v1/admin/coupon")
	adminCouponGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create coupon
		adminCouponGroupRouter.POST("/", adminCoupon.CreateCouponHandler(serverCtx))

		// Update coupon
		adminCouponGroupRouter.PUT("/", adminCoupon.UpdateCouponHandler(serverCtx))

		// Delete coupon
		adminCouponGroupRouter.DELETE("/", adminCoupon.DeleteCouponHandler(serverCtx))

		// Batch delete coupon
		adminCouponGroupRouter.DELETE("/batch", adminCoupon.BatchDeleteCouponHandler(serverCtx))

		// Get coupon list
		adminCouponGroupRouter.GET("/list", adminCoupon.GetCouponListHandler(serverCtx))
	}
}

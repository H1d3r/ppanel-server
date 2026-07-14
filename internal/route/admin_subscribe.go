package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminSubscribe "github.com/perfect-panel/server/internal/handler/admin/subscribe"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminSubscribeRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminSubscribeGroupRouter := router.Group("/v1/admin/subscribe")
	adminSubscribeGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create subscribe
		adminSubscribeGroupRouter.POST("/", adminSubscribe.CreateSubscribeHandler(serverCtx))

		// Update subscribe
		adminSubscribeGroupRouter.PUT("/", adminSubscribe.UpdateSubscribeHandler(serverCtx))

		// Delete subscribe
		adminSubscribeGroupRouter.DELETE("/", adminSubscribe.DeleteSubscribeHandler(serverCtx))

		// Batch delete subscribe
		adminSubscribeGroupRouter.DELETE("/batch", adminSubscribe.BatchDeleteSubscribeHandler(serverCtx))

		// Get subscribe details
		adminSubscribeGroupRouter.GET("/details", adminSubscribe.GetSubscribeDetailsHandler(serverCtx))

		// Create subscribe group
		adminSubscribeGroupRouter.POST("/group", adminSubscribe.CreateSubscribeGroupHandler(serverCtx))

		// Update subscribe group
		adminSubscribeGroupRouter.PUT("/group", adminSubscribe.UpdateSubscribeGroupHandler(serverCtx))

		// Delete subscribe group
		adminSubscribeGroupRouter.DELETE("/group", adminSubscribe.DeleteSubscribeGroupHandler(serverCtx))

		// Batch delete subscribe group
		adminSubscribeGroupRouter.DELETE("/group/batch", adminSubscribe.BatchDeleteSubscribeGroupHandler(serverCtx))

		// Get subscribe group list
		adminSubscribeGroupRouter.GET("/group/list", adminSubscribe.GetSubscribeGroupListHandler(serverCtx))

		// Get subscribe list
		adminSubscribeGroupRouter.GET("/list", adminSubscribe.GetSubscribeListHandler(serverCtx))

		// Reset all subscribe tokens
		adminSubscribeGroupRouter.POST("/reset_all_token", adminSubscribe.ResetAllSubscribeTokenHandler(serverCtx))

		// Subscribe sort
		adminSubscribeGroupRouter.POST("/sort", adminSubscribe.SubscribeSortHandler(serverCtx))
	}
}

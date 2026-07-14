package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminApplication "github.com/perfect-panel/server/internal/handler/admin/application"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminApplicationRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminApplicationGroupRouter := router.Group("/v1/admin/application")
	adminApplicationGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create subscribe application
		adminApplicationGroupRouter.POST("/", adminApplication.CreateSubscribeApplicationHandler(serverCtx))

		// Preview Template
		adminApplicationGroupRouter.GET("/preview", adminApplication.PreviewSubscribeTemplateHandler(serverCtx))

		// Update subscribe application
		adminApplicationGroupRouter.PUT("/subscribe_application", adminApplication.UpdateSubscribeApplicationHandler(serverCtx))

		// Delete subscribe application
		adminApplicationGroupRouter.DELETE("/subscribe_application", adminApplication.DeleteSubscribeApplicationHandler(serverCtx))

		// Get subscribe application list
		adminApplicationGroupRouter.GET("/subscribe_application_list", adminApplication.GetSubscribeApplicationListHandler(serverCtx))
	}
}

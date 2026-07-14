package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminAnnouncement "github.com/perfect-panel/server/internal/handler/admin/announcement"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminAnnouncementRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminAnnouncementGroupRouter := router.Group("/v1/admin/announcement")
	adminAnnouncementGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create announcement
		adminAnnouncementGroupRouter.POST("/", adminAnnouncement.CreateAnnouncementHandler(serverCtx))

		// Update announcement
		adminAnnouncementGroupRouter.PUT("/", adminAnnouncement.UpdateAnnouncementHandler(serverCtx))

		// Delete announcement
		adminAnnouncementGroupRouter.DELETE("/", adminAnnouncement.DeleteAnnouncementHandler(serverCtx))

		// Get announcement
		adminAnnouncementGroupRouter.GET("/detail", adminAnnouncement.GetAnnouncementHandler(serverCtx))

		// Get announcement list
		adminAnnouncementGroupRouter.GET("/list", adminAnnouncement.GetAnnouncementListHandler(serverCtx))
	}
}

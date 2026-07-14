package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicAnnouncement "github.com/perfect-panel/server/internal/handler/public/announcement"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicAnnouncementRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicAnnouncementGroupRouter := router.Group("/v1/public/announcement")
	publicAnnouncementGroupRouter.Use(middleware.AuthMiddleware(serverCtx), middleware.DeviceMiddleware(serverCtx))
	publicAnnouncementGroupRouter.GET("/list", publicAnnouncement.QueryAnnouncementHandler(serverCtx))
}

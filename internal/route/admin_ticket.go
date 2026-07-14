package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminTicket "github.com/perfect-panel/server/internal/handler/admin/ticket"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminTicketRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminTicketGroupRouter := router.Group("/v1/admin/ticket")
	adminTicketGroupRouter.Use(middleware.AuthMiddleware(serverCtx))
	{
		adminTicketGroupRouter.PUT("/", adminTicket.UpdateTicketStatusHandler(serverCtx))
		adminTicketGroupRouter.GET("/detail", adminTicket.GetTicketHandler(serverCtx))
		adminTicketGroupRouter.POST("/follow", adminTicket.CreateTicketFollowHandler(serverCtx))
		adminTicketGroupRouter.GET("/list", adminTicket.GetTicketListHandler(serverCtx))
	}
}

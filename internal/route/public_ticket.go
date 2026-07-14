package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicTicket "github.com/perfect-panel/server/internal/handler/public/ticket"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicTicketRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicTicketGroupRouter := router.Group("/v1/public/ticket")
	publicTicketGroupRouter.Use(middleware.AuthMiddleware(serverCtx), middleware.DeviceMiddleware(serverCtx))
	publicTicketGroupRouter.PUT("/", publicTicket.UpdateUserTicketStatusHandler(serverCtx))
	publicTicketGroupRouter.POST("/", publicTicket.CreateUserTicketHandler(serverCtx))
	publicTicketGroupRouter.GET("/detail", publicTicket.GetUserTicketDetailsHandler(serverCtx))
	publicTicketGroupRouter.POST("/follow", publicTicket.CreateUserTicketFollowHandler(serverCtx))
	publicTicketGroupRouter.GET("/list", publicTicket.GetUserTicketListHandler(serverCtx))
}

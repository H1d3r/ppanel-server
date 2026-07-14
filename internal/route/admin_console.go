package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminConsole "github.com/perfect-panel/server/internal/handler/admin/console"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminConsoleRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminConsoleGroupRouter := router.Group("/v1/admin/console")
	adminConsoleGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Query revenue statistics
		adminConsoleGroupRouter.GET("/revenue", adminConsole.QueryRevenueStatisticsHandler(serverCtx))

		// Query server total data
		adminConsoleGroupRouter.GET("/server", adminConsole.QueryServerTotalDataHandler(serverCtx))

		// Query ticket wait reply
		adminConsoleGroupRouter.GET("/ticket", adminConsole.QueryTicketWaitReplyHandler(serverCtx))

		// Query user statistics
		adminConsoleGroupRouter.GET("/user", adminConsole.QueryUserStatisticsHandler(serverCtx))
	}
}

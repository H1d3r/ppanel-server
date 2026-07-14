package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminLog "github.com/perfect-panel/server/internal/handler/admin/log"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminLogRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminLogGroupRouter := router.Group("/v1/admin/log")
	adminLogGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Filter balance log
		adminLogGroupRouter.GET("/balance/list", adminLog.FilterBalanceLogHandler(serverCtx))

		// Filter commission log
		adminLogGroupRouter.GET("/commission/list", adminLog.FilterCommissionLogHandler(serverCtx))

		// Filter email log
		adminLogGroupRouter.GET("/email/list", adminLog.FilterEmailLogHandler(serverCtx))

		// Filter gift log
		adminLogGroupRouter.GET("/gift/list", adminLog.FilterGiftLogHandler(serverCtx))

		// Filter login log
		adminLogGroupRouter.GET("/login/list", adminLog.FilterLoginLogHandler(serverCtx))

		// Get message log list
		adminLogGroupRouter.GET("/message/list", adminLog.GetMessageLogListHandler(serverCtx))

		// Filter mobile log
		adminLogGroupRouter.GET("/mobile/list", adminLog.FilterMobileLogHandler(serverCtx))

		// Filter register log
		adminLogGroupRouter.GET("/register/list", adminLog.FilterRegisterLogHandler(serverCtx))

		// Filter server traffic log
		adminLogGroupRouter.GET("/server/traffic/list", adminLog.FilterServerTrafficLogHandler(serverCtx))

		// Get log setting
		adminLogGroupRouter.GET("/setting", adminLog.GetLogSettingHandler(serverCtx))

		// Update log setting
		adminLogGroupRouter.POST("/setting", adminLog.UpdateLogSettingHandler(serverCtx))

		// Filter subscribe log
		adminLogGroupRouter.GET("/subscribe/list", adminLog.FilterSubscribeLogHandler(serverCtx))

		// Filter reset subscribe log
		adminLogGroupRouter.GET("/subscribe/reset/list", adminLog.FilterResetSubscribeLogHandler(serverCtx))

		// Filter user subscribe traffic log
		adminLogGroupRouter.GET("/subscribe/traffic/list", adminLog.FilterUserSubscribeTrafficLogHandler(serverCtx))

		// Filter traffic log details
		adminLogGroupRouter.GET("/traffic/details", adminLog.FilterTrafficLogDetailsHandler(serverCtx))
	}
}

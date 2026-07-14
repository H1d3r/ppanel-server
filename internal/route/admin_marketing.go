package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminMarketing "github.com/perfect-panel/server/internal/handler/admin/marketing"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminMarketingRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminMarketingGroupRouter := router.Group("/v1/admin/marketing")
	adminMarketingGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Get batch send email task list
		adminMarketingGroupRouter.GET("/email/batch/list", adminMarketing.GetBatchSendEmailTaskListHandler(serverCtx))

		// Get pre-send email count
		adminMarketingGroupRouter.POST("/email/batch/pre-send-count", adminMarketing.GetPreSendEmailCountHandler(serverCtx))

		// Create a batch send email task
		adminMarketingGroupRouter.POST("/email/batch/send", adminMarketing.CreateBatchSendEmailTaskHandler(serverCtx))

		// Get batch send email task status
		adminMarketingGroupRouter.POST("/email/batch/status", adminMarketing.GetBatchSendEmailTaskStatusHandler(serverCtx))

		// Stop a batch send email task
		adminMarketingGroupRouter.POST("/email/batch/stop", adminMarketing.StopBatchSendEmailTaskHandler(serverCtx))

		// Create a quota task
		adminMarketingGroupRouter.POST("/quota/create", adminMarketing.CreateQuotaTaskHandler(serverCtx))

		// Query quota task list
		adminMarketingGroupRouter.GET("/quota/list", adminMarketing.QueryQuotaTaskListHandler(serverCtx))

		// Query quota task pre-count
		adminMarketingGroupRouter.POST("/quota/pre-count", adminMarketing.QueryQuotaTaskPreCountHandler(serverCtx))
	}
}

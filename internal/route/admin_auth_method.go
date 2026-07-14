package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminAuthMethod "github.com/perfect-panel/server/internal/handler/admin/authMethod"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminAuthMethodRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminAuthMethodGroupRouter := router.Group("/v1/admin/auth-method")
	adminAuthMethodGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Get auth method config
		adminAuthMethodGroupRouter.GET("/config", adminAuthMethod.GetAuthMethodConfigHandler(serverCtx))

		// Update auth method config
		adminAuthMethodGroupRouter.PUT("/config", adminAuthMethod.UpdateAuthMethodConfigHandler(serverCtx))

		// Get email support platform
		adminAuthMethodGroupRouter.GET("/email_platform", adminAuthMethod.GetEmailPlatformHandler(serverCtx))

		// Get auth method list
		adminAuthMethodGroupRouter.GET("/list", adminAuthMethod.GetAuthMethodListHandler(serverCtx))

		// Get sms support platform
		adminAuthMethodGroupRouter.GET("/sms_platform", adminAuthMethod.GetSmsPlatformHandler(serverCtx))

		// Test email send
		adminAuthMethodGroupRouter.POST("/test_email_send", adminAuthMethod.TestEmailSendHandler(serverCtx))

		// Test sms send
		adminAuthMethodGroupRouter.POST("/test_sms_send", adminAuthMethod.TestSmsSendHandler(serverCtx))
	}
}

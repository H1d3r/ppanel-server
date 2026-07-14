package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	common "github.com/perfect-panel/server/internal/handler/common"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerCommonRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	commonGroupRouter := router.Group("/v1/common")
	commonGroupRouter.Use(middleware.DeviceMiddleware(serverCtx))
	{
		commonGroupRouter.GET("/ads", common.GetAdsHandler(serverCtx))
		commonGroupRouter.POST("/check_verification_code", common.CheckVerificationCodeHandler(serverCtx))
		commonGroupRouter.GET("/client", common.GetClientHandler(serverCtx))
		commonGroupRouter.GET("/heartbeat", common.HeartbeatHandler(serverCtx))
		commonGroupRouter.POST("/send_code", common.SendEmailCodeHandler(serverCtx))
		commonGroupRouter.POST("/send_sms_code", common.SendSmsCodeHandler(serverCtx))
		commonGroupRouter.GET("/site/config", common.GetGlobalConfigHandler(serverCtx))
		commonGroupRouter.GET("/site/privacy", common.GetPrivacyPolicyHandler(serverCtx))
		commonGroupRouter.GET("/site/stat", common.GetStatHandler(serverCtx))
		commonGroupRouter.GET("/site/tos", common.GetTosHandler(serverCtx))
	}
}

package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminSystem "github.com/perfect-panel/server/internal/handler/admin/system"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminSystemRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminSystemGroupRouter := router.Group("/v1/admin/system")
	adminSystemGroupRouter.Use(middleware.AuthMiddleware(serverCtx))
	{
		adminSystemGroupRouter.GET("/currency_config", adminSystem.GetCurrencyConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/currency_config", adminSystem.UpdateCurrencyConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/get_node_multiplier", adminSystem.GetNodeMultiplierHandler(serverCtx))
		adminSystemGroupRouter.GET("/invite_config", adminSystem.GetInviteConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/invite_config", adminSystem.UpdateInviteConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/module", adminSystem.GetModuleConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/node_config", adminSystem.GetNodeConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/node_config", adminSystem.UpdateNodeConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/node_multiplier/preview", adminSystem.PreViewNodeMultiplierHandler(serverCtx))
		adminSystemGroupRouter.GET("/privacy", adminSystem.GetPrivacyPolicyConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/privacy", adminSystem.UpdatePrivacyPolicyConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/register_config", adminSystem.GetRegisterConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/register_config", adminSystem.UpdateRegisterConfigHandler(serverCtx))
		adminSystemGroupRouter.POST("/set_node_multiplier", adminSystem.SetNodeMultiplierHandler(serverCtx))
		adminSystemGroupRouter.POST("/setting_telegram_bot", adminSystem.SettingTelegramBotHandler(serverCtx))
		adminSystemGroupRouter.GET("/site_config", adminSystem.GetSiteConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/site_config", adminSystem.UpdateSiteConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/subscribe_config", adminSystem.GetSubscribeConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/subscribe_config", adminSystem.UpdateSubscribeConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/tos_config", adminSystem.GetTosConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/tos_config", adminSystem.UpdateTosConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/verify_code_config", adminSystem.GetVerifyCodeConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/verify_code_config", adminSystem.UpdateVerifyCodeConfigHandler(serverCtx))
		adminSystemGroupRouter.GET("/verify_config", adminSystem.GetVerifyConfigHandler(serverCtx))
		adminSystemGroupRouter.PUT("/verify_config", adminSystem.UpdateVerifyConfigHandler(serverCtx))
	}
}

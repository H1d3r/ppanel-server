package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicUser "github.com/perfect-panel/server/internal/handler/public/user"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicUserRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicUserGroupRouter := router.Group("/v1/public/user")
	publicUserGroupRouter.Use(middleware.AuthMiddleware(serverCtx), middleware.DeviceMiddleware(serverCtx))
	publicUserGroupRouter.GET("/affiliate/count", publicUser.QueryUserAffiliateHandler(serverCtx))
	publicUserGroupRouter.GET("/affiliate/list", publicUser.QueryUserAffiliateListHandler(serverCtx))
	publicUserGroupRouter.GET("/balance_log", publicUser.QueryUserBalanceLogHandler(serverCtx))
	publicUserGroupRouter.PUT("/bind_email", publicUser.UpdateBindEmailHandler(serverCtx))
	publicUserGroupRouter.PUT("/bind_mobile", publicUser.UpdateBindMobileHandler(serverCtx))
	publicUserGroupRouter.POST("/bind_oauth", publicUser.BindOAuthHandler(serverCtx))
	publicUserGroupRouter.POST("/bind_oauth/callback", publicUser.BindOAuthCallbackHandler(serverCtx))
	publicUserGroupRouter.GET("/bind_telegram", publicUser.BindTelegramHandler(serverCtx))
	publicUserGroupRouter.GET("/commission_log", publicUser.QueryUserCommissionLogHandler(serverCtx))
	publicUserGroupRouter.POST("/commission_withdraw", publicUser.CommissionWithdrawHandler(serverCtx))
	publicUserGroupRouter.GET("/devices", publicUser.GetDeviceListHandler(serverCtx))
	publicUserGroupRouter.GET("/info", publicUser.QueryUserInfoHandler(serverCtx))
	publicUserGroupRouter.GET("/login_log", publicUser.GetLoginLogHandler(serverCtx))
	publicUserGroupRouter.PUT("/notify", publicUser.UpdateUserNotifyHandler(serverCtx))
	publicUserGroupRouter.GET("/oauth_methods", publicUser.GetOAuthMethodsHandler(serverCtx))
	publicUserGroupRouter.PUT("/password", publicUser.UpdateUserPasswordHandler(serverCtx))
	publicUserGroupRouter.PUT("/rules", publicUser.UpdateUserRulesHandler(serverCtx))
	publicUserGroupRouter.GET("/subscribe", publicUser.QueryUserSubscribeHandler(serverCtx))
	publicUserGroupRouter.GET("/subscribe_log", publicUser.GetSubscribeLogHandler(serverCtx))
	publicUserGroupRouter.PUT("/subscribe_note", publicUser.UpdateUserSubscribeNoteHandler(serverCtx))
	publicUserGroupRouter.PUT("/subscribe_token", publicUser.ResetUserSubscribeTokenHandler(serverCtx))
	publicUserGroupRouter.PUT("/unbind_device", publicUser.UnbindDeviceHandler(serverCtx))
	publicUserGroupRouter.POST("/unbind_oauth", publicUser.UnbindOAuthHandler(serverCtx))
	publicUserGroupRouter.POST("/unbind_telegram", publicUser.UnbindTelegramHandler(serverCtx))
	publicUserGroupRouter.POST("/unsubscribe", publicUser.UnsubscribeHandler(serverCtx))
	publicUserGroupRouter.POST("/unsubscribe/pre", publicUser.PreUnsubscribeHandler(serverCtx))
	publicUserGroupRouter.POST("/verify_email", publicUser.VerifyEmailHandler(serverCtx))
	publicUserGroupRouter.GET("/withdrawal_log", publicUser.QueryWithdrawalLogHandler(serverCtx))
}

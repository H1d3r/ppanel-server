package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminUser "github.com/perfect-panel/server/internal/handler/admin/user"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminUserRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminUserGroupRouter := router.Group("/v1/admin/user")
	adminUserGroupRouter.Use(middleware.AuthMiddleware(serverCtx))
	{
		adminUserGroupRouter.DELETE("/", adminUser.DeleteUserHandler(serverCtx))
		adminUserGroupRouter.POST("/", adminUser.CreateUserHandler(serverCtx))
		adminUserGroupRouter.POST("/auth_method", adminUser.CreateUserAuthMethodHandler(serverCtx))
		adminUserGroupRouter.DELETE("/auth_method", adminUser.DeleteUserAuthMethodHandler(serverCtx))
		adminUserGroupRouter.PUT("/auth_method", adminUser.UpdateUserAuthMethodHandler(serverCtx))
		adminUserGroupRouter.GET("/auth_method", adminUser.GetUserAuthMethodHandler(serverCtx))
		adminUserGroupRouter.PUT("/basic", adminUser.UpdateUserBasicInfoHandler(serverCtx))
		adminUserGroupRouter.DELETE("/batch", adminUser.BatchDeleteUserHandler(serverCtx))
		adminUserGroupRouter.GET("/current", adminUser.CurrentUserHandler(serverCtx))
		adminUserGroupRouter.GET("/detail", adminUser.GetUserDetailHandler(serverCtx))
		adminUserGroupRouter.PUT("/device", adminUser.UpdateUserDeviceHandler(serverCtx))
		adminUserGroupRouter.DELETE("/device", adminUser.DeleteUserDeviceHandler(serverCtx))
		adminUserGroupRouter.PUT("/device/kick_offline", adminUser.KickOfflineByUserDeviceHandler(serverCtx))
		adminUserGroupRouter.GET("/list", adminUser.GetUserListHandler(serverCtx))
		adminUserGroupRouter.GET("/login/logs", adminUser.GetUserLoginLogsHandler(serverCtx))
		adminUserGroupRouter.PUT("/notify", adminUser.UpdateUserNotifySettingHandler(serverCtx))
		adminUserGroupRouter.GET("/subscribe", adminUser.GetUserSubscribeHandler(serverCtx))
		adminUserGroupRouter.POST("/subscribe", adminUser.CreateUserSubscribeHandler(serverCtx))
		adminUserGroupRouter.PUT("/subscribe", adminUser.UpdateUserSubscribeHandler(serverCtx))
		adminUserGroupRouter.DELETE("/subscribe", adminUser.DeleteUserSubscribeHandler(serverCtx))
		adminUserGroupRouter.GET("/subscribe/detail", adminUser.GetUserSubscribeByIdHandler(serverCtx))
		adminUserGroupRouter.GET("/subscribe/device", adminUser.GetUserSubscribeDevicesHandler(serverCtx))
		adminUserGroupRouter.GET("/subscribe/logs", adminUser.GetUserSubscribeLogsHandler(serverCtx))
		adminUserGroupRouter.GET("/subscribe/reset/logs", adminUser.GetUserSubscribeResetTrafficLogsHandler(serverCtx))
		adminUserGroupRouter.POST("/subscribe/reset/token", adminUser.ResetUserSubscribeTokenHandler(serverCtx))
		adminUserGroupRouter.POST("/subscribe/reset/traffic", adminUser.ResetUserSubscribeTrafficHandler(serverCtx))
		adminUserGroupRouter.POST("/subscribe/toggle", adminUser.ToggleUserSubscribeStatusHandler(serverCtx))
		adminUserGroupRouter.GET("/subscribe/traffic_logs", adminUser.GetUserSubscribeTrafficLogsHandler(serverCtx))
	}
}

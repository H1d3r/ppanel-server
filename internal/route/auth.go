package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	auth "github.com/perfect-panel/server/internal/handler/auth"
	authOauth "github.com/perfect-panel/server/internal/handler/auth/oauth"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAuthRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	authGroupRouter := router.Group("/v1/auth")
	authGroupRouter.Use(middleware.DeviceMiddleware(serverCtx))
	{
		authGroupRouter.GET("/check", auth.CheckUserHandler(serverCtx))
		authGroupRouter.GET("/check/telephone", auth.CheckUserTelephoneHandler(serverCtx))
		authGroupRouter.POST("/login", auth.UserLoginHandler(serverCtx))
		authGroupRouter.POST("/login/device", auth.DeviceLoginHandler(serverCtx))
		authGroupRouter.POST("/login/telephone", auth.TelephoneLoginHandler(serverCtx))
		authGroupRouter.POST("/register", auth.UserRegisterHandler(serverCtx))
		authGroupRouter.POST("/register/telephone", auth.TelephoneUserRegisterHandler(serverCtx))
		authGroupRouter.POST("/reset", auth.ResetPasswordHandler(serverCtx))
		authGroupRouter.POST("/reset/telephone", auth.TelephoneResetPasswordHandler(serverCtx))
	}

	authOauthGroupRouter := router.Group("/v1/auth/oauth")
	{
		authOauthGroupRouter.POST("/callback/apple", authOauth.AppleLoginCallbackHandler(serverCtx))
		authOauthGroupRouter.POST("/login", authOauth.OAuthLoginHandler(serverCtx))
		authOauthGroupRouter.POST("/login/token", authOauth.OAuthLoginGetTokenHandler(serverCtx))
	}
}

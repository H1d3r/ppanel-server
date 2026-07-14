package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	publicDocument "github.com/perfect-panel/server/internal/handler/public/document"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerPublicDocumentRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	publicDocumentGroupRouter := router.Group("/v1/public/document")
	publicDocumentGroupRouter.Use(middleware.AuthMiddleware(serverCtx), middleware.DeviceMiddleware(serverCtx))
	publicDocumentGroupRouter.GET("/detail", publicDocument.QueryDocumentDetailHandler(serverCtx))
	publicDocumentGroupRouter.GET("/list", publicDocument.QueryDocumentListHandler(serverCtx))
}

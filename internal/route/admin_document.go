package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminDocument "github.com/perfect-panel/server/internal/handler/admin/document"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminDocumentRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminDocumentGroupRouter := router.Group("/v1/admin/document")
	adminDocumentGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create document
		adminDocumentGroupRouter.POST("/", adminDocument.CreateDocumentHandler(serverCtx))

		// Update document
		adminDocumentGroupRouter.PUT("/", adminDocument.UpdateDocumentHandler(serverCtx))

		// Delete document
		adminDocumentGroupRouter.DELETE("/", adminDocument.DeleteDocumentHandler(serverCtx))

		// Batch delete document
		adminDocumentGroupRouter.DELETE("/batch", adminDocument.BatchDeleteDocumentHandler(serverCtx))

		// Get document detail
		adminDocumentGroupRouter.GET("/detail", adminDocument.GetDocumentDetailHandler(serverCtx))

		// Get document list
		adminDocumentGroupRouter.GET("/list", adminDocument.GetDocumentListHandler(serverCtx))
	}
}

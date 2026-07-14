package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	adminAds "github.com/perfect-panel/server/internal/handler/admin/ads"
	"github.com/perfect-panel/server/internal/middleware"
	"github.com/perfect-panel/server/internal/svc"
)

func registerAdminAdsRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	adminAdsGroupRouter := router.Group("/v1/admin/ads")
	adminAdsGroupRouter.Use(middleware.AuthMiddleware(serverCtx))

	{
		// Create Ads
		adminAdsGroupRouter.POST("/", adminAds.CreateAdsHandler(serverCtx))

		// Update Ads
		adminAdsGroupRouter.PUT("/", adminAds.UpdateAdsHandler(serverCtx))

		// Delete Ads
		adminAdsGroupRouter.DELETE("/", adminAds.DeleteAdsHandler(serverCtx))

		// Get Ads Detail
		adminAdsGroupRouter.GET("/detail", adminAds.GetAdsDetailHandler(serverCtx))

		// Get Ads List
		adminAdsGroupRouter.GET("/list", adminAds.GetAdsListHandler(serverCtx))
	}
}

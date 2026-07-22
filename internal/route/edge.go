package route

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	edge "github.com/perfect-panel/server/internal/handler/edge"
	"github.com/perfect-panel/server/internal/svc"
)

func registerEdgeRoutes(router *server.Hertz, serverCtx *svc.ServiceContext) {
	if !serverCtx.Config.EdgeSubscribe.Enabled {
		return
	}
	router.GET("/api/edge/v1/manifest", edge.ManifestHandler(serverCtx))
}

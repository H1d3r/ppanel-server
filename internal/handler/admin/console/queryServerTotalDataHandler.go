package console

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/console"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// QueryServerTotalDataHandler documents Query server total data.
//
// @Summary Query server total data
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.ServerTotalDataResponse}
// @Router /v1/admin/console/server [get]
func QueryServerTotalDataHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := console.NewQueryServerTotalDataLogic(ctx, svcCtx)
		resp, err := l.QueryServerTotalData()
		result.HttpResult(c, resp, err)
	}
}

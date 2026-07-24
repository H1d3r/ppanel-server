package console

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// QueryRevenueStatisticsHandler documents Query revenue statistics.
//
// @Summary Query revenue statistics
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.RevenueStatisticsResponse}
// @Router /v1/admin/console/revenue [get]
func QueryRevenueStatisticsHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Platform.QueryRevenueStatistics(ctx)
		result.HttpResult(c, resp, err)
	}
}

package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetNodeMultiplierHandler documents Get Node Multiplier.
//
// @Summary Get Node Multiplier
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetNodeMultiplierResponse}
// @Router /v1/admin/system/get_node_multiplier [get]
func GetNodeMultiplierHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Platform.GetNodeMultiplier(ctx)
		result.HttpResult(c, resp, err)
	}
}

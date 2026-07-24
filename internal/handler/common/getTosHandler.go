package common

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetTosHandler documents Get Tos Content.
//
// @Summary Get Tos Content
// @Tags common
// @Produce json
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetTosResponse}
// @Router /v1/common/site/tos [get]
func GetTosHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Platform.GetTos(ctx)
		result.HttpResult(c, resp, err)
	}
}

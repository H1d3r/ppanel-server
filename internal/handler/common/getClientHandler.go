package common

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetClientHandler documents Get Client.
//
// @Summary Get Client
// @Tags common
// @Produce json
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetSubscribeClientResponse}
// @Router /v1/common/client [get]
func GetClientHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		resp, err := svcCtx.Platform.GetClient(ctx)
		result.HttpResult(c, resp, err)
	}
}

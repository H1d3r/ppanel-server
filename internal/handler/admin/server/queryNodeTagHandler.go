package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// QueryNodeTagHandler documents Query all node tags.
//
// @Summary Query all node tags
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.QueryNodeTagResponse}
// @Router /v1/admin/server/node/tags [get]
func QueryNodeTagHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		resp, err := svcCtx.Network.QueryNodeTag(c)
		result.HttpResult(ctx, resp, err)
	}
}

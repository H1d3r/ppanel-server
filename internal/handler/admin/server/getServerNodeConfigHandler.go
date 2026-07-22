package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/server"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// GetServerNodeConfigHandler documents Get Server Node Config.
//
// @Summary Get Server Node Config
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetServerNodeConfigRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetServerNodeConfigResponse}
// @Router /v1/admin/server/node_config [get]
func GetServerNodeConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.GetServerNodeConfigRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		l := server.NewGetServerNodeConfigLogic(c, svcCtx)
		resp, err := l.GetServerNodeConfig(&req)
		result.HttpResult(ctx, resp, err)
	}
}

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

// FilterServerListHandler documents Filter Server List.
//
// @Summary Filter Server List
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.FilterServerListRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.FilterServerListResponse}
// @Router /v1/admin/server/list [get]
func FilterServerListHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.FilterServerListRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		l := server.NewFilterServerListLogic(c, svcCtx)
		resp, err := l.FilterServerList(&req)
		result.HttpResult(ctx, resp, err)
	}
}

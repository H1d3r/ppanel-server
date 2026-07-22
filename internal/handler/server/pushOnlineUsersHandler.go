package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/server"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
)

// PushOnlineUsersHandler documents Push online users.
//
// @Summary Push online users
// @Tags node
// @Accept json
// @Produce json
// @Security NodeSecret
// @Param request body dto.OnlineUsersRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/server/online [post]
func PushOnlineUsersHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		req := dto.OnlineUsersRequest{}
		_ = ctx.BindJSON(&req)
		commonReq, err := serverCommonRequest(ctx)
		if err != nil {
			writeParamError(ctx, err)
			return
		}
		req.ServerCommon = commonReq
		if validateErr := svcCtx.Validate(&req); validateErr != nil {
			writeParamError(ctx, validateErr)
			return
		}

		l := server.NewPushOnlineUsersLogic(c, svcCtx)
		writeHTTPResult(ctx, nil, l.PushOnlineUsers(&req))
	}
}

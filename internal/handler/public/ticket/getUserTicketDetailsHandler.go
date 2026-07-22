package ticket

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/public/ticket"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// GetUserTicketDetailsHandler documents Get ticket detail.
//
// @Summary Get ticket detail
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetUserTicketDetailRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.Ticket}
// @Router /v1/public/ticket/detail [get]
func GetUserTicketDetailsHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.GetUserTicketDetailRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		l := ticket.NewGetUserTicketDetailsLogic(c, svcCtx)
		resp, err := l.GetUserTicketDetails(&req)
		result.HttpResult(ctx, resp, err)
	}
}

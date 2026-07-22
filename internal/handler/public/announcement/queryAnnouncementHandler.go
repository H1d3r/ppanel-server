package announcement

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/public/announcement"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// QueryAnnouncementHandler documents Query announcement.
//
// @Summary Query announcement
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.QueryAnnouncementRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.QueryAnnouncementResponse}
// @Router /v1/public/announcement/list [get]
func QueryAnnouncementHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.QueryAnnouncementRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			result.ParamErrorResult(ctx, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(ctx, validateErr)
			return
		}

		l := announcement.NewQueryAnnouncementLogic(c, svcCtx)
		resp, err := l.QueryAnnouncement(&req)
		result.HttpResult(ctx, resp, err)
	}
}

package announcement

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/announcement"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// GetAnnouncementHandler documents Get announcement.
//
// @Summary Get announcement
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetAnnouncementRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.Announcement}
// @Router /v1/admin/announcement/detail [get]
func GetAnnouncementHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.GetAnnouncementRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := announcement.NewGetAnnouncementLogic(ctx, svcCtx)
		resp, err := l.GetAnnouncement(&req)
		result.HttpResult(c, resp, err)
	}
}

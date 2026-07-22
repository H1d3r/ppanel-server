package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/public/user"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetDeviceListHandler documents Get Device List.
//
// @Summary Get Device List
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetDeviceListResponse}
// @Router /v1/public/user/devices [get]
func GetDeviceListHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		l := user.NewGetDeviceListLogic(c, svcCtx)
		resp, err := l.GetDeviceList()
		result.HttpResult(ctx, resp, err)
	}
}

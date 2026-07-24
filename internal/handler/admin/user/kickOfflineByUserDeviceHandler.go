package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// KickOfflineByUserDeviceHandler documents kick offline user device.
//
// @Summary kick offline user device
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.KickOfflineRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/admin/user/device/kick_offline [put]
func KickOfflineByUserDeviceHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.KickOfflineRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		err := svcCtx.Identity.KickOfflineByUserDevice(ctx, &req)
		result.HttpResult(c, nil, err)
	}
}
